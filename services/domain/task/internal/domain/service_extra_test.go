package domain_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/teamboard/services/domain/task/internal/domain"
)

// ── ListTasks ─────────────────────────────────────────────────────────────────

func TestTaskService_ListTasks_UnknownBoard(t *testing.T) {
	svc, _, _, _ := makeService(t)
	tasks, cursor, err := (*svc).ListTasks(context.Background(), uuid.New(), uuid.New(),
		domain.TaskFilter{}, domain.Pagination{Limit: 10})
	require.NoError(t, err)
	assert.Empty(t, tasks)
	assert.Nil(t, cursor)
}

func TestTaskService_ListTasks_Pagination(t *testing.T) {
	_, repo, proj, _ := makeService(t)
	boardID, projectID, userID := uuid.New(), uuid.New(), uuid.New()
	seedBoard(repo, boardID, projectID)
	proj.allow(projectID, userID, "task:read", "task:create")

	svc2 := domain.NewTaskService(repo, proj, newFakeDocClient())
	// Create 5 tasks.
	for i := 0; i < 5; i++ {
		_, err := svc2.CreateTask(context.Background(), userID, domain.CreateTaskInput{
			BoardID: boardID,
			Title:   "task",
			Labels:  []string{},
		})
		require.NoError(t, err)
	}

	tasks, cursor, err := svc2.ListTasks(context.Background(), boardID, userID,
		domain.TaskFilter{}, domain.Pagination{Limit: 3})
	require.NoError(t, err)
	assert.Len(t, tasks, 3)
	assert.NotNil(t, cursor)
}

func TestTaskService_ListTasks_ZeroLimit_Defaults(t *testing.T) {
	_, repo, proj, _ := makeService(t)
	task := seedTask(repo, proj)
	proj.allow(task.ProjectID, task.CreatedBy, "task:read", "task:create")

	svc2 := domain.NewTaskService(repo, proj, newFakeDocClient())
	tasks, _, err := svc2.ListTasks(context.Background(), task.BoardID, task.CreatedBy,
		domain.TaskFilter{}, domain.Pagination{Limit: 0})
	require.NoError(t, err)
	assert.Len(t, tasks, 1)
}

// ── UpdateTask ────────────────────────────────────────────────────────────────

func TestTaskService_UpdateTask_NotFound(t *testing.T) {
	svc, _, _, _ := makeService(t)
	title := "x"
	_, err := (*svc).UpdateTask(context.Background(), uuid.New(), uuid.New(), domain.TaskPatch{Title: &title})
	assertDomainError(t, err, "task_not_found")
}

func TestTaskService_UpdateTask_DueDateClear(t *testing.T) {
	svc, repo, proj, _ := makeService(t)
	task := seedTask(repo, proj)

	due := time.Now().Add(24 * time.Hour)
	task.DueDate = &due
	repo.tasks[task.ID] = task

	updated, err := (*svc).UpdateTask(context.Background(), task.ID, task.CreatedBy, domain.TaskPatch{
		DueDateSet: true,
		DueDate:    nil,
	})
	require.NoError(t, err)
	assert.Nil(t, updated.DueDate)
}

func TestTaskService_UpdateTask_Labels(t *testing.T) {
	svc, repo, proj, _ := makeService(t)
	task := seedTask(repo, proj)

	labels := []string{"alpha", "beta"}
	updated, err := (*svc).UpdateTask(context.Background(), task.ID, task.CreatedBy, domain.TaskPatch{
		Labels: &labels,
	})
	require.NoError(t, err)
	assert.Equal(t, labels, updated.Labels)
}

// ── DeleteTask ────────────────────────────────────────────────────────────────

func TestTaskService_DeleteTask_NotFound(t *testing.T) {
	svc, _, _, _ := makeService(t)
	err := (*svc).DeleteTask(context.Background(), uuid.New(), uuid.New())
	assertDomainError(t, err, "task_not_found")
}

func TestTaskService_DeleteTask_PermissionDenied(t *testing.T) {
	svc, repo, proj, _ := makeService(t)
	task := seedTask(repo, proj)
	// requester has no task:delete permission
	proj.allow(task.ProjectID, task.CreatedBy, "task:read")

	err := (*svc).DeleteTask(context.Background(), task.ID, task.CreatedBy)
	assertDomainError(t, err, "permission_denied")
}

// ── MoveTask ──────────────────────────────────────────────────────────────────

func TestTaskService_MoveTask_NotFound(t *testing.T) {
	svc, _, _, _ := makeService(t)
	_, err := (*svc).MoveTask(context.Background(), uuid.New(), uuid.New(), uuid.New(), nil, nil)
	assertDomainError(t, err, "task_not_found")
}

func TestTaskService_MoveTask_ColumnNotInBoard(t *testing.T) {
	svc, repo, proj, _ := makeService(t)
	task := seedTask(repo, proj)
	otherCol := uuid.New()
	seedColumn(repo, otherCol, uuid.New(), "Other Board Col")

	_, err := (*svc).MoveTask(context.Background(), task.ID, task.CreatedBy, otherCol, nil, nil)
	assertDomainError(t, err, "column_not_in_board")
}

func TestTaskService_MoveTask_SameStatus_NoStatusEvent(t *testing.T) {
	svc, repo, proj, _ := makeService(t)
	task := seedTask(repo, proj)
	// Two columns with same "done" name → same derived status.
	col1 := uuid.New()
	col2 := uuid.New()
	seedColumn(repo, col1, task.BoardID, "Done")
	seedColumn(repo, col2, task.BoardID, "Done2") // "Done2" also derives Done
	proj.allow(task.ProjectID, task.CreatedBy, "task:read", "task:create", "task:update")

	// First move to col1 (Done).
	repo.outbox = nil
	_, err := (*svc).MoveTask(context.Background(), task.ID, task.CreatedBy, col1, nil, nil)
	require.NoError(t, err)
	firstCount := len(collectEventTypes(repo))

	// Move again within same derived-status column.
	repo.outbox = nil
	_, err = (*svc).MoveTask(context.Background(), task.ID, task.CreatedBy, col2, nil, nil)
	require.NoError(t, err)
	secondTypes := collectEventTypes(repo)
	assert.Len(t, secondTypes, firstCount-1, "no status.changed event expected for same-derived status")
	assert.NotContains(t, secondTypes, "task.status.changed")
}

// ── CreateComment ─────────────────────────────────────────────────────────────

func TestTaskService_CreateComment_TaskNotFound(t *testing.T) {
	svc, _, _, _ := makeService(t)
	_, err := (*svc).CreateComment(context.Background(), uuid.New(), uuid.New(), "body")
	assertDomainError(t, err, "task_not_found")
}

func TestTaskService_CreateComment_BodyTooLong_Fails(t *testing.T) {
	svc, repo, proj, _ := makeService(t)
	task := seedTask(repo, proj)
	proj.allow(task.ProjectID, task.CreatedBy, "comment:create")

	_, err := (*svc).CreateComment(context.Background(), task.ID, task.CreatedBy, strings.Repeat("x", 10001))
	assertDomainError(t, err, "validation_failed")
}

// ── ListComments ──────────────────────────────────────────────────────────────

func TestTaskService_ListComments_TaskNotFound(t *testing.T) {
	svc, _, _, _ := makeService(t)
	_, err := (*svc).ListComments(context.Background(), uuid.New(), uuid.New())
	assertDomainError(t, err, "task_not_found")
}

// ── UpdateComment ─────────────────────────────────────────────────────────────

func TestTaskService_UpdateComment_NotFound(t *testing.T) {
	svc, _, _, _ := makeService(t)
	_, err := (*svc).UpdateComment(context.Background(), uuid.New(), uuid.New(), "body")
	assertDomainError(t, err, "comment_not_found")
}

func TestTaskService_UpdateComment_ModeratorCanEdit(t *testing.T) {
	svc, repo, proj, _ := makeService(t)
	task := seedTask(repo, proj)
	proj.allow(task.ProjectID, task.CreatedBy, "task:read", "task:create", "comment:create", "comment:read")

	comment, err := (*svc).CreateComment(context.Background(), task.ID, task.CreatedBy, "original")
	require.NoError(t, err)

	moderator := uuid.New()
	proj.allow(task.ProjectID, moderator, "task:delete")
	repo.outbox = nil

	updated, err := (*svc).UpdateComment(context.Background(), comment.ID, moderator, "moderated")
	require.NoError(t, err)
	assert.Equal(t, "moderated", updated.Body)
}

func TestTaskService_UpdateComment_EmptyBody_Fails(t *testing.T) {
	svc, repo, proj, _ := makeService(t)
	task := seedTask(repo, proj)
	proj.allow(task.ProjectID, task.CreatedBy, "task:read", "task:create", "comment:create")

	comment, err := (*svc).CreateComment(context.Background(), task.ID, task.CreatedBy, "ok")
	require.NoError(t, err)

	_, err = (*svc).UpdateComment(context.Background(), comment.ID, task.CreatedBy, "")
	assertDomainError(t, err, "validation_failed")
}

// ── DeleteComment ─────────────────────────────────────────────────────────────

func TestTaskService_DeleteComment_NotFound(t *testing.T) {
	svc, _, _, _ := makeService(t)
	err := (*svc).DeleteComment(context.Background(), uuid.New(), uuid.New())
	assertDomainError(t, err, "comment_not_found")
}

func TestTaskService_DeleteComment_ModeratorCanDelete(t *testing.T) {
	svc, repo, proj, _ := makeService(t)
	task := seedTask(repo, proj)
	proj.allow(task.ProjectID, task.CreatedBy, "task:read", "task:create", "comment:create")

	comment, err := (*svc).CreateComment(context.Background(), task.ID, task.CreatedBy, "to moderate")
	require.NoError(t, err)

	moderator := uuid.New()
	proj.allow(task.ProjectID, moderator, "task:delete")

	err = (*svc).DeleteComment(context.Background(), comment.ID, moderator)
	require.NoError(t, err)
}

func TestTaskService_DeleteComment_NonAuthorNonModerator_Fails(t *testing.T) {
	svc, repo, proj, _ := makeService(t)
	task := seedTask(repo, proj)
	proj.allow(task.ProjectID, task.CreatedBy, "task:read", "task:create", "comment:create")

	comment, err := (*svc).CreateComment(context.Background(), task.ID, task.CreatedBy, "mine")
	require.NoError(t, err)

	stranger := uuid.New()
	err = (*svc).DeleteComment(context.Background(), comment.ID, stranger)
	assertDomainError(t, err, "not_comment_author")
}

// ── AddAttachment ─────────────────────────────────────────────────────────────

func TestTaskService_AddAttachment_DocumentUnreachable(t *testing.T) {
	svc, repo, proj, _ := makeService(t)
	task := seedTask(repo, proj)
	proj.allow(task.ProjectID, task.CreatedBy, "task:read", "task:create", "task:update")

	_, err := (*svc).AddAttachment(context.Background(), task.ID, task.CreatedBy, uuid.New())
	assertDomainError(t, err, "document_unreachable")
}

// ── ListAttachments ───────────────────────────────────────────────────────────

func TestTaskService_ListAttachments_TaskNotFound(t *testing.T) {
	svc, _, _, _ := makeService(t)
	_, err := (*svc).ListAttachments(context.Background(), uuid.New(), uuid.New())
	assertDomainError(t, err, "task_not_found")
}

// ── RemoveAttachment ──────────────────────────────────────────────────────────

func TestTaskService_RemoveAttachment_NotFound(t *testing.T) {
	svc, _, _, _ := makeService(t)
	err := (*svc).RemoveAttachment(context.Background(), uuid.New(), uuid.New())
	assertDomainError(t, err, "attachment_not_found")
}

// ── GetTaskHistory ────────────────────────────────────────────────────────────

func TestTaskService_GetTaskHistory_NotFound(t *testing.T) {
	svc, _, _, _ := makeService(t)
	_, err := (*svc).GetTaskHistory(context.Background(), uuid.New(), uuid.New())
	assertDomainError(t, err, "task_not_found")
}

// ── Status derivation ─────────────────────────────────────────────────────────

func TestDeriveStatus_VariousNames(t *testing.T) {
	cases := []struct {
		name   string
		expect domain.Status
	}{
		{"Done", domain.StatusDone},
		{"done tasks", domain.StatusDone},
		{"In Progress", domain.StatusInProgress},
		{"progress", domain.StatusInProgress},
		{"Blocked", domain.StatusBlocked},
		{"blocked by legal", domain.StatusBlocked},
		{"Backlog", domain.StatusOpen},
		{"review", domain.StatusOpen},
	}
	for _, tc := range cases {
		got := domain.DeriveStatus(tc.name)
		assert.Equal(t, tc.expect, got, "name=%q", tc.name)
	}
}

// ── CreateTask status from column ─────────────────────────────────────────────

func TestTaskService_CreateTask_DoneColumnSetsStatus(t *testing.T) {
	svc, repo, proj, _ := makeService(t)
	boardID, projectID, userID := uuid.New(), uuid.New(), uuid.New()
	seedBoard(repo, boardID, projectID)
	colID := uuid.New()
	seedColumn(repo, colID, boardID, "Done")
	proj.allow(projectID, userID, "task:create")

	task, err := (*svc).CreateTask(context.Background(), userID, domain.CreateTaskInput{
		BoardID:  boardID,
		ColumnID: &colID,
		Title:    "finished task",
		Labels:   []string{},
	})
	require.NoError(t, err)
	assert.Equal(t, domain.StatusDone, task.Status)
}

func TestTaskService_CreateTask_NoColumn_OpenStatus(t *testing.T) {
	svc, repo, proj, _ := makeService(t)
	boardID, projectID, userID := uuid.New(), uuid.New(), uuid.New()
	seedBoard(repo, boardID, projectID)
	proj.allow(projectID, userID, "task:create")

	task, err := (*svc).CreateTask(context.Background(), userID, domain.CreateTaskInput{
		BoardID: boardID,
		Title:   "no column",
		Labels:  []string{},
	})
	require.NoError(t, err)
	assert.Equal(t, domain.StatusOpen, task.Status)
}

// ── Mention extraction ────────────────────────────────────────────────────────

func TestExtractMentionHandles(t *testing.T) {
	handles := domain.ExtractMentionHandles("Hello @alice and @bob.smith@example.com, also @alice again")
	assert.Equal(t, []string{"alice", "bob.smith@example.com"}, handles)
}

func TestExtractMentionHandles_NoMentions(t *testing.T) {
	handles := domain.ExtractMentionHandles("no mentions here")
	assert.Empty(t, handles)
}

// ── Comment with UUID mention ─────────────────────────────────────────────────

func TestTaskService_CreateComment_WithUUIDMention(t *testing.T) {
	svc, repo, proj, _ := makeService(t)
	task := seedTask(repo, proj)
	proj.allow(task.ProjectID, task.CreatedBy, "task:read", "task:create", "comment:create")

	mentionedUser := uuid.New()
	repo.knownUsers[mentionedUser] = "someone@test.com"

	body := "Hey @" + mentionedUser.String() + " check this out"
	comment, err := (*svc).CreateComment(context.Background(), task.ID, task.CreatedBy, body)
	require.NoError(t, err)
	assert.Contains(t, comment.Mentions, mentionedUser)
}

// ── ValidStatus ───────────────────────────────────────────────────────────────

func TestValidStatus(t *testing.T) {
	assert.True(t, domain.ValidStatus(domain.StatusOpen))
	assert.True(t, domain.ValidStatus(domain.StatusInProgress))
	assert.True(t, domain.ValidStatus(domain.StatusDone))
	assert.True(t, domain.ValidStatus(domain.StatusBlocked))
	assert.True(t, domain.ValidStatus(domain.StatusArchived))
	assert.False(t, domain.ValidStatus("unknown"))
}

// ── Error helpers ─────────────────────────────────────────────────────────────

func TestDomainError_Formatting(t *testing.T) {
	e := &domain.Error{Code: "test_code", Message: "test message"}
	assert.Equal(t, "test_code: test message", e.Error())
	assert.Equal(t, "test_code", e.GetCode())
	assert.Nil(t, e.Unwrap())
}

func TestDomainError_WithCause(t *testing.T) {
	cause := &domain.Error{Code: "inner", Message: "inner msg"}
	e := &domain.Error{Code: "outer", Message: "outer msg", Cause: cause}
	assert.Contains(t, e.Error(), "inner")
	assert.Equal(t, cause, e.Unwrap())
}

// ── buildUpdateDiff ───────────────────────────────────────────────────────────

func TestTaskService_UpdateTask_DescriptionChange(t *testing.T) {
	svc, repo, proj, _ := makeService(t)
	task := seedTask(repo, proj)

	newDesc := "new description"
	updated, err := (*svc).UpdateTask(context.Background(), task.ID, task.CreatedBy, domain.TaskPatch{
		Description: &newDesc,
	})
	require.NoError(t, err)
	assert.Equal(t, "new description", updated.Description)
}

// ── RemoveAttachment task not found ──────────────────────────────────────────

func TestTaskService_RemoveAttachment_TaskNotFound(t *testing.T) {
	svc, repo, _, _ := makeService(t)
	// Insert a dangling attachment whose task does not exist.
	attID := uuid.New()
	repo.attachments[attID] = &domain.Attachment{
		ID: attID, TaskID: uuid.New(), DocumentID: uuid.New(),
		AddedBy: uuid.New(), AddedAt: time.Now(),
	}

	err := (*svc).RemoveAttachment(context.Background(), attID, uuid.New())
	assertDomainError(t, err, "task_not_found")
}

// ── GetTask ───────────────────────────────────────────────────────────────────

func TestTaskService_GetTask_PermissionDenied(t *testing.T) {
	svc, repo, proj, _ := makeService(t)
	task := seedTask(repo, proj)
	// strip read permission
	proj.perms[task.ProjectID.String()+":"+task.CreatedBy.String()] = &domain.PermissionSet{
		IsMember: true, ProjectExists: true, Permissions: []string{},
	}

	_, err := (*svc).GetTask(context.Background(), task.ID, task.CreatedBy)
	assertDomainError(t, err, "permission_denied")
}
