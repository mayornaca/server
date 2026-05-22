package posts

import (
	"errors"
	"fmt"
	"time"

	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/db"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type PostsSummary struct {
	Total    int            `json:"total"`
	ByStatus map[string]int `json:"byStatus"`
}

var ErrNotFound = errors.New("post not found")

type ServiceParams struct {
	fx.In

	IDGen db.IDGen

	Posts *Repository

	Logger *zap.Logger
}

type Service struct {
	idgen db.IDGen

	posts *Repository

	logger *zap.Logger
}

func NewService(params ServiceParams) *Service {
	return &Service{
		idgen:  params.IDGen,
		posts:  params.Posts,
		logger: params.Logger,
	}
}

func (s *Service) Select(userID string, filters ...SelectFilter) ([]*SosPost, error) {
	filters = append(filters, WithUserID(userID))
	items, err := s.posts.Select(filters...)
	if err != nil {
		return nil, fmt.Errorf("failed to select posts: %w", err)
	}
	return items, nil
}

func (s *Service) Get(userID string, id string) (*SosPost, error) {
	post, err := s.posts.SelectOne(WithUserID(userID), WithID(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get post: %w", err)
	}
	return post, nil
}

func (s *Service) Create(userID string, post *SosPost) error {
	if post.ID == "" {
		post.ID = s.idgen()
	}
	post.UserID = userID
	if post.Status == "" {
		post.Status = PostStatusUnknown
	}

	if err := s.posts.Insert(post); err != nil {
		return fmt.Errorf("failed to create post: %w", err)
	}
	return nil
}

func (s *Service) Update(userID string, id string, post *SosPost) error {
	existing, err := s.Get(userID, id)
	if err != nil {
		return err
	}

	existing.Name = post.Name
	existing.KmMarker = post.KmMarker
	existing.PhoneNumber = post.PhoneNumber
	// cloud-gesvial.19.1: deliberately do NOT copy Disabled here. The DTO is
	// a `*SosPost` with bool zero-value semantics, so a form that omits the
	// field (PostFormDialog.tsx sends only name/km/phone) would unmarshal
	// to Disabled=false and un-disable the post on every save. The
	// disabled flag is mutated exclusively through PATCH /posts/:id, which
	// uses a `*bool` in the PostPatch DTO and can therefore tell "absent"
	// from "false". The actual operator-reported "no se desactiva" bug was
	// the Upsert overwriting Disabled on every Android sync, fixed in
	// posts/repository.go.
	if post.Status != "" {
		existing.Status = post.Status
	}
	if post.LastTestDate != nil {
		existing.LastTestDate = post.LastTestDate
	}

	if err := s.posts.Update(existing); err != nil {
		return fmt.Errorf("failed to update post: %w", err)
	}
	return nil
}

// UpdateLastTestDate writes only the last_test_date column. Used by the tests
// service to bump the timestamp from a goroutine without racing the PATCH
// handler. cloud-gesvial.19.1.
func (s *Service) UpdateLastTestDate(userID string, id string, t time.Time) error {
	// Verify ownership / admin:all before issuing the surgical update.
	if _, err := s.Get(userID, id); err != nil {
		return err
	}
	if err := s.posts.UpdateFields(id, map[string]any{
		"last_test_date": t,
	}); err != nil {
		return fmt.Errorf("failed to bump lastTestDate: %w", err)
	}
	return nil
}

// UpdateStatus writes only the status column (and bumps last_test_date in the
// same statement so the panel reflects "last completed at" without a second
// round-trip). Used after Test.Report promotes/demotes a post status.
// cloud-gesvial.19.1.
func (s *Service) UpdateStatus(userID string, id string, status PostStatus, completedAt time.Time) error {
	if _, err := s.Get(userID, id); err != nil {
		return err
	}
	if err := s.posts.UpdateFields(id, map[string]any{
		"status":         status,
		"last_test_date": completedAt,
	}); err != nil {
		return fmt.Errorf("failed to update post status: %w", err)
	}
	return nil
}

// PatchFields holds optional fields for partial updates.
type PatchFields struct {
	Name        *string
	KmMarker    *string
	PhoneNumber *string
	Status      *PostStatus
	Disabled    *bool
}

func (s *Service) Patch(userID string, id string, fields PatchFields) (*SosPost, error) {
	existing, err := s.Get(userID, id)
	if err != nil {
		return nil, err
	}

	if fields.Name != nil {
		existing.Name = *fields.Name
	}
	if fields.KmMarker != nil {
		existing.KmMarker = *fields.KmMarker
	}
	if fields.PhoneNumber != nil {
		existing.PhoneNumber = *fields.PhoneNumber
	}
	if fields.Status != nil {
		existing.Status = *fields.Status
	}
	if fields.Disabled != nil {
		existing.Disabled = *fields.Disabled
	}

	if err := s.posts.Update(existing); err != nil {
		return nil, fmt.Errorf("failed to patch post: %w", err)
	}
	return existing, nil
}

func (s *Service) Delete(userID string, id string) error {
	if err := s.posts.Delete(WithUserID(userID), WithID(id)); err != nil {
		return fmt.Errorf("failed to delete post: %w", err)
	}
	return nil
}

// Summary returns aggregated post counts. cloud-gesvial.20.1: gains an
// `includeDisabled` flag that matches CountByStatus' contract. The handler
// `/posts/summary` exposes it via `?includeDisabled=true` (default false),
// so the dashboard sees a consistent Total = sum(byStatus) without disabled
// posts inflating Total but not the buckets.
func (s *Service) Summary(userID string, includeDisabled bool) (*PostsSummary, error) {
	byStatus, err := s.posts.CountByStatus(userID, includeDisabled)
	if err != nil {
		return nil, fmt.Errorf("failed to get posts summary: %w", err)
	}

	total := 0
	for _, c := range byStatus {
		total += c
	}

	return &PostsSummary{
		Total:    total,
		ByStatus: byStatus,
	}, nil
}

func (s *Service) Sync(userID string, posts []*SosPost) error {
	for _, p := range posts {
		if p.ID == "" {
			p.ID = s.idgen()
		}
		p.UserID = userID
		if p.Status == "" {
			p.Status = PostStatusUnknown
		}
	}

	if err := s.posts.Upsert(posts); err != nil {
		return fmt.Errorf("failed to sync posts: %w", err)
	}
	return nil
}
