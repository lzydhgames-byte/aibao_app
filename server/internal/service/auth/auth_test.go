package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aibao/server/internal/model"
	apperr "github.com/aibao/server/internal/pkg/errors"
	"github.com/aibao/server/internal/pkg/jwtauth"
	"github.com/aibao/server/internal/pkg/safehash"
)

type fakeUserRepo struct {
	byHash map[string]*model.User
	nextID int64
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{byHash: map[string]*model.User{}, nextID: 1}
}

func (f *fakeUserRepo) CreateOrGet(_ context.Context, u *model.User) (*model.User, bool, error) {
	if existing, ok := f.byHash[u.PhoneHash]; ok {
		return existing, false, nil
	}
	u.ID = f.nextID
	f.nextID++
	f.byHash[u.PhoneHash] = u
	return u, true, nil
}

func (f *fakeUserRepo) FindByID(_ context.Context, id int64) (*model.User, error) {
	for _, u := range f.byHash {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, errors.New("not found")
}

type fakeCodeStore struct {
	saved    map[string]string
	cooldown bool
}

func (f *fakeCodeStore) Save(_ context.Context, ph, code string, _, _ time.Duration) error {
	if f.cooldown {
		return ErrCooldown
	}
	if f.saved == nil {
		f.saved = map[string]string{}
	}
	f.saved[ph] = code
	return nil
}
func (f *fakeCodeStore) Take(_ context.Context, ph string) (string, error) {
	c, ok := f.saved[ph]
	if !ok {
		return "", ErrCodeNotFound
	}
	delete(f.saved, ph)
	return c, nil
}

type fakeSMS struct {
	sent      bool
	lastPhone string
	lastCode  string
}

func (f *fakeSMS) SendCode(_ context.Context, phone, code string) error {
	f.sent = true
	f.lastPhone = phone
	f.lastCode = code
	return nil
}

type fakePhoneCipher struct{}

func (fakePhoneCipher) Encrypt(s string) ([]byte, error) { return []byte("enc:" + s), nil }
func (fakePhoneCipher) Decrypt(b []byte) (string, error) { return string(b)[4:], nil }

func newSvc(t *testing.T) (*Service, *fakeUserRepo, *fakeCodeStore, *fakeSMS) {
	t.Helper()
	repo := newFakeUserRepo()
	cs := &fakeCodeStore{}
	sms := &fakeSMS{}
	jwt := jwtauth.New("secret-x", time.Hour, 7*24*time.Hour)
	hasher := safehash.New("salt")
	svc := New(Deps{
		Users:        repo,
		CodeStore:    cs,
		SMS:          sms,
		JWT:          jwt,
		PhoneCipher:  fakePhoneCipher{},
		Hasher:       hasher,
		FixedDevCode: "123456",
		CodeTTL:      5 * time.Minute,
		Cooldown:     time.Minute,
	})
	return svc, repo, cs, sms
}

func TestSendSMS_HappyPath(t *testing.T) {
	svc, _, cs, sms := newSvc(t)
	err := svc.SendSMS(context.Background(), "13800138000")
	require.NoError(t, err)
	assert.True(t, sms.sent)
	assert.Equal(t, "13800138000", sms.lastPhone)
	assert.Equal(t, "123456", sms.lastCode)
	require.Len(t, cs.saved, 1)
}

func TestSendSMS_RejectsBadPhone(t *testing.T) {
	svc, _, _, _ := newSvc(t)
	err := svc.SendSMS(context.Background(), "abc")
	require.Error(t, err)
	ae, ok := apperr.AsAppError(err)
	require.True(t, ok)
	assert.Equal(t, apperr.CodeInvalidArgument, ae.Code)
}

func TestSendSMS_Cooldown(t *testing.T) {
	svc, _, cs, _ := newSvc(t)
	cs.cooldown = true
	err := svc.SendSMS(context.Background(), "13800138000")
	ae, _ := apperr.AsAppError(err)
	require.NotNil(t, ae)
	assert.Equal(t, apperr.CodeRateLimited, ae.Code)
}

func TestLoginOrRegister_NewUser(t *testing.T) {
	svc, repo, cs, _ := newSvc(t)
	require.NoError(t, svc.SendSMS(context.Background(), "13800138000"))

	out, err := svc.LoginOrRegister(context.Background(), "13800138000", "123456", "妈妈")
	require.NoError(t, err)
	assert.NotEmpty(t, out.AccessToken)
	assert.NotEmpty(t, out.RefreshToken)
	assert.Equal(t, "妈妈", out.User.Nickname)
	assert.Len(t, repo.byHash, 1, "exactly one user")
	assert.Empty(t, cs.saved, "code consumed")
}

func TestLoginOrRegister_ExistingUserKeepsNickname(t *testing.T) {
	svc, repo, _, _ := newSvc(t)
	require.NoError(t, svc.SendSMS(context.Background(), "13800138000"))
	_, err := svc.LoginOrRegister(context.Background(), "13800138000", "123456", "妈妈")
	require.NoError(t, err)

	require.NoError(t, svc.SendSMS(context.Background(), "13800138000"))
	out, err := svc.LoginOrRegister(context.Background(), "13800138000", "123456", "")
	require.NoError(t, err)
	assert.Equal(t, "妈妈", out.User.Nickname, "nickname not overwritten on second login")
	assert.Len(t, repo.byHash, 1)
}

func TestLoginOrRegister_DefaultNickname(t *testing.T) {
	svc, _, _, _ := newSvc(t)
	require.NoError(t, svc.SendSMS(context.Background(), "13800138000"))
	out, err := svc.LoginOrRegister(context.Background(), "13800138000", "123456", "")
	require.NoError(t, err)
	assert.Equal(t, "家长", out.User.Nickname)
}

func TestLoginOrRegister_CodeMismatch(t *testing.T) {
	svc, _, _, _ := newSvc(t)
	require.NoError(t, svc.SendSMS(context.Background(), "13800138000"))
	_, err := svc.LoginOrRegister(context.Background(), "13800138000", "999999", "妈妈")
	ae, _ := apperr.AsAppError(err)
	require.NotNil(t, ae)
	assert.Equal(t, apperr.CodeUnauthenticated, ae.Code)
	assert.Equal(t, "code_mismatch", ae.Reason)
}

func TestLoginOrRegister_NoCodeStored(t *testing.T) {
	svc, _, _, _ := newSvc(t)
	_, err := svc.LoginOrRegister(context.Background(), "13800138000", "123456", "")
	ae, _ := apperr.AsAppError(err)
	require.NotNil(t, ae)
	assert.Equal(t, apperr.CodeUnauthenticated, ae.Code)
	assert.Equal(t, "code_expired", ae.Reason)
}

func TestValidateAccess_HappyPath(t *testing.T) {
	svc, _, _, _ := newSvc(t)
	require.NoError(t, svc.SendSMS(context.Background(), "13800138000"))
	out, _ := svc.LoginOrRegister(context.Background(), "13800138000", "123456", "妈妈")

	uid, err := svc.ValidateAccess(out.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, out.User.ID, uid)
}

func TestValidateAccess_BadToken(t *testing.T) {
	svc, _, _, _ := newSvc(t)
	_, err := svc.ValidateAccess("not-a-jwt")
	require.Error(t, err)
}
