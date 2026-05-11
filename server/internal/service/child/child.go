// Package child implements the child-profile CRUD service.
package child

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/aibao/server/internal/model"
	apperr "github.com/aibao/server/internal/pkg/errors"
	"github.com/aibao/server/internal/repository"
)

// Service implements the child-profile CRUD logic.
type Service struct {
	repo repository.ChildRepo
}

// New constructs a Service.
func New(r repository.ChildRepo) *Service { return &Service{repo: r} }

// CreateInput is the user-facing input for Create.
type CreateInput struct {
	Nickname string
	Gender   string
	Birthday time.Time
}

// UpdateInput holds optional fields for Update. nil means "don't change".
type UpdateInput struct {
	Nickname *string
	Gender   *string
	Birthday *time.Time
	Profile  *[]byte // Plan 6: BOOTSTRAP-rendered profile JSON. nil = don't touch.
}

var validGenders = map[string]bool{"boy": true, "girl": true, "unspecified": true}

// Create inserts a new child for userID.
func (s *Service) Create(ctx context.Context, userID int64, in CreateInput) (*model.Child, error) {
	nick := strings.TrimSpace(in.Nickname)
	if nick == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "invalid_nickname", "孩子昵称不能为空")
	}
	if !validGenders[in.Gender] {
		return nil, apperr.New(apperr.CodeInvalidArgument, "invalid_gender", "性别必须是 boy / girl / unspecified")
	}
	if in.Birthday.IsZero() {
		return nil, apperr.New(apperr.CodeInvalidArgument, "invalid_birthday", "生日不能为空")
	}
	c := &model.Child{
		UserID:   userID,
		Nickname: nick,
		Gender:   in.Gender,
		Birthday: in.Birthday,
		Profile:  []byte(`{}`),
	}
	if err := s.repo.Create(ctx, c); err != nil {
		if errors.Is(err, repository.ErrAlreadyExists) {
			return nil, apperr.New(apperr.CodeInvalidArgument, "child_already_exists", "您已经创建过孩子档案")
		}
		return nil, apperr.Wrap(err, apperr.CodeInternal, "child_create_failed", "服务暂时不可用")
	}
	return c, nil
}

// ListByUser returns the user's child as a one- or zero-element slice.
func (s *Service) ListByUser(ctx context.Context, userID int64) ([]*model.Child, error) {
	c, err := s.repo.FindByUserID(ctx, userID)
	if errors.Is(err, repository.ErrNotFound) {
		return []*model.Child{}, nil
	}
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "child_list_failed", "服务暂时不可用")
	}
	return []*model.Child{c}, nil
}

// GetByID returns the child by id, enforcing ownership by userID.
func (s *Service) GetByID(ctx context.Context, userID, childID int64) (*model.Child, error) {
	c, err := s.repo.FindByID(ctx, childID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, apperr.New(apperr.CodeNotFound, "child_not_found", "未找到该孩子档案")
	}
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "child_load_failed", "服务暂时不可用")
	}
	if c.UserID != userID {
		return nil, apperr.New(apperr.CodePermissionDenied, "not_owner", "无权查看该孩子档案")
	}
	return c, nil
}

// Update mutates fields of an existing child belonging to userID.
func (s *Service) Update(ctx context.Context, userID, childID int64, in UpdateInput) (*model.Child, error) {
	c, err := s.repo.FindByID(ctx, childID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, apperr.New(apperr.CodeNotFound, "child_not_found", "未找到该孩子档案")
	}
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "child_load_failed", "服务暂时不可用")
	}
	if c.UserID != userID {
		return nil, apperr.New(apperr.CodePermissionDenied, "not_owner", "无权修改该孩子档案")
	}
	if in.Nickname != nil {
		nick := strings.TrimSpace(*in.Nickname)
		if nick == "" {
			return nil, apperr.New(apperr.CodeInvalidArgument, "invalid_nickname", "孩子昵称不能为空")
		}
		c.Nickname = nick
	}
	if in.Gender != nil {
		if !validGenders[*in.Gender] {
			return nil, apperr.New(apperr.CodeInvalidArgument, "invalid_gender", "性别必须是 boy / girl / unspecified")
		}
		c.Gender = *in.Gender
	}
	if in.Birthday != nil {
		c.Birthday = *in.Birthday
	}
	if in.Profile != nil {
		c.Profile = *in.Profile
	}
	if err := s.repo.Update(ctx, c); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "child_update_failed", "服务暂时不可用")
	}
	return c, nil
}
