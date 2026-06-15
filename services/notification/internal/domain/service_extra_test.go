package domain_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/teamboard/services/notification/internal/domain"
)

// Tests targeting the cursor-decoding path of ListNotifications.

func TestListNotifications_ValidCursor(t *testing.T) {
	repo := newFakeRepo()
	userID := uuid.New()
	for i := 0; i < 4; i++ {
		seedNotification(repo, userID)
	}

	svc := makeService(repo)
	// First page.
	ns1, cursor, err := svc.ListNotifications(context.Background(), userID, domain.ListFilter{Limit: 2})
	require.NoError(t, err)
	require.NotNil(t, cursor)
	require.Len(t, ns1, 2)

	// Encode cursor the way the handler would.
	raw, err := json.Marshal(cursor)
	require.NoError(t, err)
	encoded := base64.StdEncoding.EncodeToString(raw)

	// Second page with encoded cursor.
	ns2, _, err := svc.ListNotifications(context.Background(), userID, domain.ListFilter{
		Limit:  2,
		Cursor: &encoded,
	})
	require.NoError(t, err)
	assert.NotNil(t, ns2)
}

func TestListNotifications_InvalidCursorBase64(t *testing.T) {
	repo := newFakeRepo()
	svc := makeService(repo)
	bad := "not-valid-base64!!!"
	_, _, err := svc.ListNotifications(context.Background(), uuid.New(), domain.ListFilter{
		Limit:  10,
		Cursor: &bad,
	})
	var de *domain.Error
	require.True(t, errors.As(err, &de))
}

func TestListNotifications_InvalidCursorJSON(t *testing.T) {
	repo := newFakeRepo()
	svc := makeService(repo)
	// Valid base64 but not valid PageCursor JSON.
	bad := base64.StdEncoding.EncodeToString([]byte(`not json`))
	_, _, err := svc.ListNotifications(context.Background(), uuid.New(), domain.ListFilter{
		Limit:  10,
		Cursor: &bad,
	})
	var de *domain.Error
	require.True(t, errors.As(err, &de))
}

func TestListNotifications_EmptyResult(t *testing.T) {
	repo := newFakeRepo()
	svc := makeService(repo)
	ns, cursor, err := svc.ListNotifications(context.Background(), uuid.New(), domain.ListFilter{Limit: 10})
	require.NoError(t, err)
	assert.Nil(t, cursor)
	assert.Empty(t, ns)
}

func TestGetUnreadCount_Zero(t *testing.T) {
	repo := newFakeRepo()
	svc := makeService(repo)
	count, err := svc.GetUnreadCount(context.Background(), uuid.New())
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestMarkRead_IdempotentAlreadyRead(t *testing.T) {
	repo := newFakeRepo()
	userID := uuid.New()
	n := seedNotification(repo, userID)
	already := time.Now()
	n.ReadAt = &already

	svc := makeService(repo)
	// MarkRead calls GetNotification first (which succeeds), then MarkRead on repo.
	err := svc.MarkRead(context.Background(), n.ID, userID)
	require.NoError(t, err)
}

func TestMarkAllRead_NothingToMark(t *testing.T) {
	repo := newFakeRepo()
	svc := makeService(repo)
	err := svc.MarkAllRead(context.Background(), uuid.New())
	require.NoError(t, err)
}
