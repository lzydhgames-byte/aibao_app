package child

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aibao/server/internal/model"
	apperr "github.com/aibao/server/internal/pkg/errors"
	"github.com/aibao/server/internal/repository"
)

type fakeChildRepo struct {
	byID     map[int64]*model.Child
	byUser   map[int64]int64 // user_id -> child_id
	nextID   int64
	createOK bool
}

func newFakeChildRepo() *fakeChildRepo {
	return &fakeChildRepo{
		byID:     map[int64]*model.Child{},
		byUser:   map[int64]int64{},
		nextID:   1,
		createOK: true,
	}
}

func (r *fakeChildRepo) Create(_ context.Context, c *model.Child) error {
	if _, ok := r.byUser[c.UserID]; ok {
		return repository.ErrAlreadyExists
	}
	c.ID = r.nextID
	r.nextID++
	r.byID[c.ID] = c
	r.byUser[c.UserID] = c.ID
	return nil
}

func (r *fakeChildRepo) FindByUserID(_ context.Context, userID int64) (*model.Child, error) {
	id, ok := r.byUser[userID]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return r.byID[id], nil
}

func (r *fakeChildRepo) FindByID(_ context.Context, id int64) (*model.Child, error) {
	c, ok := r.byID[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return c, nil
}

func (r *fakeChildRepo) Update(_ context.Context, c *model.Child) error {
	r.byID[c.ID] = c
	return nil
}

func newSvc() (*Service, *fakeChildRepo) {
	r := newFakeChildRepo()
	return New(r), r
}

func bday(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	require.NoError(t, err)
	return d
}

func TestCreate_HappyPath(t *testing.T) {
	svc, _ := newSvc()
	c, err := svc.Create(context.Background(), 1, CreateInput{
		Nickname: "小宇", Gender: "boy", Birthday: bday(t, "2020-08-15"),
	})
	require.NoError(t, err)
	assert.Equal(t, "小宇", c.Nickname)
	assert.NotZero(t, c.ID)
}

func TestCreate_RejectsDuplicate(t *testing.T) {
	svc, _ := newSvc()
	_, err := svc.Create(context.Background(), 1, CreateInput{Nickname: "a", Gender: "boy", Birthday: bday(t, "2020-08-15")})
	require.NoError(t, err)
	_, err = svc.Create(context.Background(), 1, CreateInput{Nickname: "b", Gender: "boy", Birthday: bday(t, "2020-08-15")})
	ae, ok := apperr.AsAppError(err)
	require.True(t, ok)
	assert.Equal(t, apperr.CodeInvalidArgument, ae.Code)
	assert.Equal(t, "child_already_exists", ae.Reason)
}

func TestCreate_RejectsInvalidGender(t *testing.T) {
	svc, _ := newSvc()
	_, err := svc.Create(context.Background(), 1, CreateInput{Nickname: "a", Gender: "alien", Birthday: bday(t, "2020-08-15")})
	ae, _ := apperr.AsAppError(err)
	require.NotNil(t, ae)
	assert.Equal(t, "invalid_gender", ae.Reason)
}

func TestCreate_RejectsBlankNickname(t *testing.T) {
	svc, _ := newSvc()
	_, err := svc.Create(context.Background(), 1, CreateInput{Nickname: "  ", Gender: "boy", Birthday: bday(t, "2020-08-15")})
	ae, _ := apperr.AsAppError(err)
	require.NotNil(t, ae)
	assert.Equal(t, "invalid_nickname", ae.Reason)
}

func TestList_EmptyAndOne(t *testing.T) {
	svc, _ := newSvc()
	got, err := svc.ListByUser(context.Background(), 1)
	require.NoError(t, err)
	assert.Len(t, got, 0)

	_, _ = svc.Create(context.Background(), 1, CreateInput{Nickname: "n", Gender: "boy", Birthday: bday(t, "2020-08-15")})
	got, err = svc.ListByUser(context.Background(), 1)
	require.NoError(t, err)
	assert.Len(t, got, 1)
}

func TestUpdate_OnlyOwnerCanUpdate(t *testing.T) {
	svc, _ := newSvc()
	c, _ := svc.Create(context.Background(), 1, CreateInput{Nickname: "n", Gender: "boy", Birthday: bday(t, "2020-08-15")})

	newName := "n2"
	_, err := svc.Update(context.Background(), 999 /* not owner */, c.ID, UpdateInput{Nickname: &newName})
	ae, _ := apperr.AsAppError(err)
	require.NotNil(t, ae)
	assert.Equal(t, apperr.CodePermissionDenied, ae.Code)
}

func TestUpdate_HappyPath(t *testing.T) {
	svc, _ := newSvc()
	c, _ := svc.Create(context.Background(), 1, CreateInput{Nickname: "n", Gender: "boy", Birthday: bday(t, "2020-08-15")})

	newName := "n2"
	got, err := svc.Update(context.Background(), 1, c.ID, UpdateInput{Nickname: &newName})
	require.NoError(t, err)
	assert.Equal(t, "n2", got.Nickname)
}

func TestUpdate_NotFound(t *testing.T) {
	svc, _ := newSvc()
	newName := "x"
	_, err := svc.Update(context.Background(), 1, 9999, UpdateInput{Nickname: &newName})
	require.Error(t, err)
	ae, ok := apperr.AsAppError(err)
	require.True(t, ok)
	assert.Equal(t, apperr.CodeNotFound, ae.Code)
}
