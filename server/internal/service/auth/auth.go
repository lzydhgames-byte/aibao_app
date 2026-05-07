package auth

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/aibao/server/internal/model"
	apperr "github.com/aibao/server/internal/pkg/errors"
	"github.com/aibao/server/internal/pkg/jwtauth"
	"github.com/aibao/server/internal/pkg/logger"
	"github.com/aibao/server/internal/pkg/safehash"
)

// PhoneCipher abstracts phone-number encryption so the service can be tested
// without depending on real AES.
type PhoneCipher interface {
	Encrypt(plain string) ([]byte, error)
	Decrypt(blob []byte) (string, error)
}

// SMS is the minimal surface auth.Service needs from the SMS gateway.
type SMS interface {
	SendCode(ctx context.Context, phone, code string) error
}

// UserRepo is the minimal surface auth.Service needs from the user repository.
// Mirrors repository.UserRepo so the service can swap in a fake.
type UserRepo interface {
	CreateOrGet(ctx context.Context, u *model.User) (*model.User, bool, error)
	FindByID(ctx context.Context, id int64) (*model.User, error)
}

// Deps groups Service dependencies.
type Deps struct {
	Users        UserRepo
	CodeStore    CodeStore
	SMS          SMS
	JWT          *jwtauth.Manager
	PhoneCipher  PhoneCipher
	Hasher       *safehash.Hasher
	FixedDevCode string        // e.g. "123456" for mock provider
	CodeTTL      time.Duration // e.g. 5 min
	Cooldown     time.Duration // e.g. 60s
}

// Service is the auth service.
type Service struct {
	d Deps
}

// New constructs the Service.
func New(d Deps) *Service { return &Service{d: d} }

// LoginOutput is what LoginOrRegister returns to callers.
type LoginOutput struct {
	AccessToken  string
	RefreshToken string
	User         *model.User
}

const defaultNickname = "家长"

var phoneRe = regexp.MustCompile(`^1[3-9]\d{9}$`)

func validatePhone(p string) bool {
	return phoneRe.MatchString(p)
}

// SendSMS issues a verification code for the phone.
func (s *Service) SendSMS(ctx context.Context, phone string) error {
	if !validatePhone(phone) {
		return apperr.New(apperr.CodeInvalidArgument, "phone_invalid", "手机号格式不正确")
	}
	hash := s.d.Hasher.HashString(phone)
	code := s.d.FixedDevCode
	if err := s.d.CodeStore.Save(ctx, hash, code, s.d.CodeTTL, s.d.Cooldown); err != nil {
		if errors.Is(err, ErrCooldown) {
			return apperr.New(apperr.CodeRateLimited, "sms_rate_limited", "请稍后再试")
		}
		return apperr.Wrap(err, apperr.CodeInternal, "code_save_failed", "短信发送失败")
	}
	if err := s.d.SMS.SendCode(ctx, phone, code); err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "sms_send_failed", "短信发送失败")
	}
	logger.FromCtx(ctx).Info("auth.sms.sent", "phone", logger.MaskPhone(phone), "phone_hash", hash)
	return nil
}

// LoginOrRegister verifies code; returns access + refresh tokens for an
// existing or freshly created user.
func (s *Service) LoginOrRegister(ctx context.Context, phone, code, nickname string) (*LoginOutput, error) {
	if !validatePhone(phone) {
		return nil, apperr.New(apperr.CodeInvalidArgument, "phone_invalid", "手机号格式不正确")
	}
	if strings.TrimSpace(code) == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "code_invalid", "验证码不能为空")
	}
	hash := s.d.Hasher.HashString(phone)

	stored, err := s.d.CodeStore.Take(ctx, hash)
	if err != nil {
		if errors.Is(err, ErrCodeNotFound) {
			return nil, apperr.New(apperr.CodeUnauthenticated, "code_expired", "验证码已过期，请重新获取")
		}
		return nil, apperr.Wrap(err, apperr.CodeInternal, "code_take_failed", "验证失败")
	}
	if stored != code {
		return nil, apperr.New(apperr.CodeUnauthenticated, "code_mismatch", "验证码错误")
	}

	enc, err := s.d.PhoneCipher.Encrypt(phone)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "phone_encrypt_failed", "服务暂时不可用")
	}

	chosenNickname := strings.TrimSpace(nickname)
	if chosenNickname == "" {
		chosenNickname = defaultNickname
	}

	u, _, err := s.d.Users.CreateOrGet(ctx, &model.User{
		PhoneHash:        hash,
		PhoneEncrypted:   enc,
		Nickname:         chosenNickname,
		SubscriptionTier: "free",
	})
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "user_upsert_failed", "服务暂时不可用")
	}

	access, err := s.d.JWT.IssueAccess(u.ID)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "jwt_issue_failed", "服务暂时不可用")
	}
	refresh, err := s.d.JWT.IssueRefresh(u.ID)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "jwt_issue_failed", "服务暂时不可用")
	}

	logger.FromCtx(ctx).Info("auth.login_or_register",
		"user_id", u.ID,
		"phone_hash", hash,
		"new_user", u.CreatedAt.After(time.Now().Add(-5*time.Second)),
	)
	return &LoginOutput{AccessToken: access, RefreshToken: refresh, User: u}, nil
}

// ValidateAccess verifies an access token and returns its user id.
func (s *Service) ValidateAccess(tok string) (int64, error) {
	c, err := s.d.JWT.ParseAccess(tok)
	if err != nil {
		return 0, fmt.Errorf("parse access: %w", err)
	}
	return c.UserID, nil
}
