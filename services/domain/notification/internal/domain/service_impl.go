package domain

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type notificationSvc struct {
	repo Repository
}

func NewNotificationService(repo Repository) NotificationService {
	return &notificationSvc{repo: repo}
}

func (s *notificationSvc) ListNotifications(ctx context.Context, userID uuid.UUID, filter ListFilter) ([]*Notification, *PageCursor, error) {
	if filter.Limit <= 0 || filter.Limit > 200 {
		filter.Limit = 50
	}

	if filter.Cursor != nil {
		raw, err := base64.StdEncoding.DecodeString(*filter.Cursor)
		if err != nil {
			return nil, nil, &Error{Code: ErrInvalidChannel.Code, Message: "invalid cursor"}
		}
		var pc PageCursor
		if err := json.Unmarshal(raw, &pc); err != nil {
			return nil, nil, &Error{Code: ErrInvalidChannel.Code, Message: "invalid cursor"}
		}
		cursorStr := pc.CreatedAt.Format(time.RFC3339Nano) + "," + pc.ID.String()
		filter.Cursor = &cursorStr
	}

	items, err := s.repo.ListNotifications(ctx, userID, ListFilter{
		UnreadOnly: filter.UnreadOnly,
		Type:       filter.Type,
		Limit:      filter.Limit + 1,
		Cursor:     filter.Cursor,
	})
	if err != nil {
		return nil, nil, err
	}

	var next *PageCursor
	if len(items) > filter.Limit {
		items = items[:filter.Limit]
		last := items[len(items)-1]
		next = &PageCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	return items, next, nil
}

func (s *notificationSvc) GetUnreadCount(ctx context.Context, userID uuid.UUID) (int, error) {
	return s.repo.CountUnread(ctx, userID)
}

func (s *notificationSvc) MarkRead(ctx context.Context, id, userID uuid.UUID) error {
	// Verify ownership
	if _, err := s.repo.GetNotification(ctx, id, userID); err != nil {
		return err
	}
	return s.repo.MarkRead(ctx, id, userID)
}

func (s *notificationSvc) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	return s.repo.MarkAllRead(ctx, userID)
}

func (s *notificationSvc) DeleteNotification(ctx context.Context, id, userID uuid.UUID) error {
	if _, err := s.repo.GetNotification(ctx, id, userID); err != nil {
		return err
	}
	return s.repo.DeleteNotification(ctx, id, userID)
}
