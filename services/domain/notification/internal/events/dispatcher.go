package events

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/teamboard/services/domain/notification/internal/domain"
	"github.com/teamboard/services/domain/notification/internal/push"
	"github.com/teamboard/services/domain/notification/internal/ws"
)

// DispatchResult describes what should happen after processing an event.
type DispatchResult struct {
	ChannelPushes []ChannelPush
	Notifications []NotificationToCreate
}

type ChannelPush struct {
	Channel domain.Channel
	Data    []byte
}

type NotificationToCreate struct {
	UserID        uuid.UUID
	Type          domain.NotificationType
	Payload       map[string]any
	ProjectID     *uuid.UUID
	SourceEventID *string
}

// Handler maps a single event type to a DispatchResult.
type Handler interface {
	Handle(ctx context.Context, env Envelope) (*DispatchResult, error)
}

// Dispatcher routes inbound events to the correct handler.
type Dispatcher struct {
	repo     domain.Repository
	pushSvc  *push.Service
	handlers map[string]Handler
}

func NewDispatcher(repo domain.Repository, pushSvc *push.Service) *Dispatcher {
	d := &Dispatcher{repo: repo, pushSvc: pushSvc, handlers: make(map[string]Handler)}
	d.register()
	return d
}

func (d *Dispatcher) register() {
	d.handlers["task.created"] = &genericChannelHandler{
		channelFn: func(p map[string]any) domain.Channel {
			return domain.BoardChannel(mustUUID(p["board_id"]))
		},
	}
	d.handlers["task.updated"] = &multiChannelHandler{
		channelFns: []channelFn{
			func(p map[string]any) domain.Channel { return domain.BoardChannel(mustUUID(p["board_id"])) },
			func(p map[string]any) domain.Channel { return domain.TaskChannel(mustUUID(p["task_id"])) },
		},
	}
	d.handlers["task.moved"] = &multiChannelHandler{
		channelFns: []channelFn{
			func(p map[string]any) domain.Channel { return domain.BoardChannel(mustUUID(p["board_id"])) },
			func(p map[string]any) domain.Channel { return domain.TaskChannel(mustUUID(p["task_id"])) },
		},
	}
	d.handlers["task.assigned"] = &taskAssignedHandler{}
	d.handlers["task.unassigned"] = &taskUnassignedHandler{}
	d.handlers["task.status.changed"] = &multiChannelHandler{
		channelFns: []channelFn{
			func(p map[string]any) domain.Channel { return domain.BoardChannel(mustUUID(p["board_id"])) },
			func(p map[string]any) domain.Channel { return domain.TaskChannel(mustUUID(p["task_id"])) },
		},
	}
	d.handlers["task.deleted"] = &taskDeletedHandler{}
	d.handlers["task.commented"] = &taskCommentedHandler{}
	d.handlers["task.attachment.added"] = &genericChannelHandler{
		channelFn: func(p map[string]any) domain.Channel { return domain.TaskChannel(mustUUID(p["task_id"])) },
	}
	d.handlers["document.uploaded"] = &genericChannelHandler{
		channelFn: func(p map[string]any) domain.Channel { return domain.ProjectChannel(mustUUID(p["project_id"])) },
	}
	d.handlers["project.member.added"] = &projectMemberAddedHandler{}
	d.handlers["project.member.removed"] = &projectMemberRemovedHandler{}
	d.handlers["project.deleted"] = &projectDeletedHandler{}
	d.handlers["project.invitation.sent"] = &projectInvitationSentHandler{}
	d.handlers["project.invitation.accepted"] = &projectInvitationAcceptedHandler{}
	d.handlers["project.invitation.declined"] = &projectInvitationDeclinedHandler{}
	d.handlers["board.created"] = &genericChannelHandler{
		channelFn: func(p map[string]any) domain.Channel { return domain.ProjectChannel(mustUUID(p["project_id"])) },
	}
}

func (d *Dispatcher) Dispatch(ctx context.Context, env Envelope) error {
	handler, ok := d.handlers[env.EventType]
	if !ok {
		return nil // forward-compatible: ignore unknown events
	}

	result, err := handler.Handle(ctx, env)
	if err != nil {
		return err
	}
	if result == nil {
		return nil
	}

	// Persist notifications with idempotency (ON CONFLICT DO NOTHING).
	for _, n := range result.Notifications {
		notif := &domain.Notification{
			ID:            uuid.New(),
			UserID:        n.UserID,
			Type:          n.Type,
			Payload:       n.Payload,
			ProjectID:     n.ProjectID,
			SourceEventID: n.SourceEventID,
		}
		saved, saveErr := d.repo.CreateNotification(ctx, notif)
		if saveErr != nil {
			slog.ErrorContext(ctx, "dispatcher: save notification", "error", saveErr)
			continue
		}
		if saved == nil {
			continue // idempotent duplicate — already delivered
		}
		// Also push the notification to the user in real-time.
		frame, _ := ws.NotificationFrame(saved.ID.String(), string(saved.Type), saved.CreatedAt, saved.Payload)
		if pushErr := d.pushSvc.BroadcastToUser(ctx, saved.UserID, frame); pushErr != nil {
			slog.ErrorContext(ctx, "dispatcher: push notification", "error", pushErr)
		}
	}

	// Broadcast channel pushes.
	for _, cp := range result.ChannelPushes {
		if pushErr := d.pushSvc.BroadcastToChannel(ctx, cp.Channel, cp.Data); pushErr != nil {
			slog.ErrorContext(ctx, "dispatcher: push channel", "channel", cp.Channel, "error", pushErr)
		}
	}

	return nil
}

// ── Generic helpers ───────────────────────────────────────────────────────────

type channelFn func(map[string]any) domain.Channel

type genericChannelHandler struct {
	channelFn channelFn
}

func (h *genericChannelHandler) Handle(_ context.Context, env Envelope) (*DispatchResult, error) {
	var p map[string]any
	if err := unmarshal(env.Payload, &p); err != nil {
		return nil, err
	}
	frame, err := ws.EventFrame(string(h.channelFn(p)), env.EventType, time.Now(), p)
	if err != nil {
		return nil, err
	}
	return &DispatchResult{ChannelPushes: []ChannelPush{{Channel: h.channelFn(p), Data: frame}}}, nil
}

type multiChannelHandler struct {
	channelFns []channelFn
}

func (h *multiChannelHandler) Handle(_ context.Context, env Envelope) (*DispatchResult, error) {
	var p map[string]any
	if err := unmarshal(env.Payload, &p); err != nil {
		return nil, err
	}
	result := &DispatchResult{}
	for _, fn := range h.channelFns {
		ch := fn(p)
		frame, _ := ws.EventFrame(string(ch), env.EventType, time.Now(), p)
		result.ChannelPushes = append(result.ChannelPushes, ChannelPush{Channel: ch, Data: frame})
	}
	return result, nil
}

// ── Task assignment handlers ──────────────────────────────────────────────────

type taskAssignedHandler struct{}

func (h *taskAssignedHandler) Handle(_ context.Context, env Envelope) (*DispatchResult, error) {
	var p struct {
		TaskID     string `json:"task_id"`
		AssigneeID string `json:"assignee_id"`
		AssignedBy string `json:"assigned_by"`
		ProjectID  string `json:"project_id"`
	}
	if err := unmarshal(env.Payload, &p); err != nil {
		return nil, err
	}
	taskID := mustUUIDStr(p.TaskID)
	ch := domain.TaskChannel(taskID)
	frame, _ := ws.EventFrame(string(ch), env.EventType, time.Now(), p)
	result := &DispatchResult{ChannelPushes: []ChannelPush{{Channel: ch, Data: frame}}}

	// Notify assignee if different from assigner.
	assignee := mustUUIDStr(p.AssigneeID)
	assigner := mustUUIDStr(p.AssignedBy)
	if assignee != assigner && assignee != uuid.Nil {
		projID := mustUUIDStr(p.ProjectID)
		sourceID := env.MessageID + ":assigned:" + p.AssigneeID
		result.Notifications = append(result.Notifications, NotificationToCreate{
			UserID:        assignee,
			Type:          domain.TypeTaskAssigned,
			Payload:       map[string]any{"task_id": p.TaskID, "assigned_by": p.AssignedBy},
			ProjectID:     uuidPtr(projID),
			SourceEventID: &sourceID,
		})
	}
	return result, nil
}

type taskUnassignedHandler struct{}

func (h *taskUnassignedHandler) Handle(_ context.Context, env Envelope) (*DispatchResult, error) {
	var p struct {
		TaskID          string `json:"task_id"`
		PreviousAssignee string `json:"previous_assignee_id"`
		UnassignedBy    string `json:"unassigned_by"`
		ProjectID       string `json:"project_id"`
	}
	if err := unmarshal(env.Payload, &p); err != nil {
		return nil, err
	}
	ch := domain.TaskChannel(mustUUIDStr(p.TaskID))
	frame, _ := ws.EventFrame(string(ch), env.EventType, time.Now(), p)
	result := &DispatchResult{ChannelPushes: []ChannelPush{{Channel: ch, Data: frame}}}

	prev := mustUUIDStr(p.PreviousAssignee)
	by := mustUUIDStr(p.UnassignedBy)
	if prev != uuid.Nil && prev != by {
		projID := mustUUIDStr(p.ProjectID)
		sourceID := env.MessageID + ":unassigned:" + p.PreviousAssignee
		result.Notifications = append(result.Notifications, NotificationToCreate{
			UserID:        prev,
			Type:          domain.TypeTaskUnassigned,
			Payload:       map[string]any{"task_id": p.TaskID, "unassigned_by": p.UnassignedBy},
			ProjectID:     uuidPtr(projID),
			SourceEventID: &sourceID,
		})
	}
	return result, nil
}

type taskDeletedHandler struct{}

func (h *taskDeletedHandler) Handle(_ context.Context, env Envelope) (*DispatchResult, error) {
	var p struct {
		TaskID     string `json:"task_id"`
		BoardID    string `json:"board_id"`
		AssigneeID string `json:"assignee_id"`
		DeletedBy  string `json:"deleted_by"`
		ProjectID  string `json:"project_id"`
	}
	if err := unmarshal(env.Payload, &p); err != nil {
		return nil, err
	}
	boardCh := domain.BoardChannel(mustUUIDStr(p.BoardID))
	taskCh := domain.TaskChannel(mustUUIDStr(p.TaskID))
	boardFrame, _ := ws.EventFrame(string(boardCh), env.EventType, time.Now(), p)
	taskFrame, _ := ws.EventFrame(string(taskCh), env.EventType, time.Now(), p)
	result := &DispatchResult{ChannelPushes: []ChannelPush{
		{Channel: boardCh, Data: boardFrame},
		{Channel: taskCh, Data: taskFrame},
	}}
	assignee := mustUUIDStr(p.AssigneeID)
	deleter := mustUUIDStr(p.DeletedBy)
	if assignee != uuid.Nil && assignee != deleter {
		projID := mustUUIDStr(p.ProjectID)
		sourceID := env.MessageID + ":deleted:" + p.AssigneeID
		result.Notifications = append(result.Notifications, NotificationToCreate{
			UserID:        assignee,
			Type:          domain.TypeTaskDeleted,
			Payload:       map[string]any{"task_id": p.TaskID, "deleted_by": p.DeletedBy},
			ProjectID:     uuidPtr(projID),
			SourceEventID: &sourceID,
		})
	}
	return result, nil
}

// ── task.commented ────────────────────────────────────────────────────────────

type taskCommentedHandler struct{}

func (h *taskCommentedHandler) Handle(_ context.Context, env Envelope) (*DispatchResult, error) {
	var p struct {
		TaskID      string   `json:"task_id"`
		CommentID   string   `json:"comment_id"`
		AuthorID    string   `json:"author_id"`
		AssigneeID  string   `json:"assignee_id"`
		ProjectID   string   `json:"project_id"`
		Mentions    []string `json:"mentions"`
		BodyExcerpt string   `json:"body_excerpt"`
	}
	if err := unmarshal(env.Payload, &p); err != nil {
		return nil, err
	}
	ch := domain.TaskChannel(mustUUIDStr(p.TaskID))
	frame, _ := ws.EventFrame(string(ch), env.EventType, time.Now(), p)
	result := &DispatchResult{ChannelPushes: []ChannelPush{{Channel: ch, Data: frame}}}

	author := mustUUIDStr(p.AuthorID)
	assignee := mustUUIDStr(p.AssigneeID)
	projID := mustUUIDStr(p.ProjectID)
	mentionSet := make(map[uuid.UUID]bool)

	for _, m := range p.Mentions {
		uid := mustUUIDStr(m)
		if uid == uuid.Nil || uid == author {
			continue
		}
		mentionSet[uid] = true
		sourceID := env.MessageID + ":mention:" + m
		result.Notifications = append(result.Notifications, NotificationToCreate{
			UserID: uid,
			Type:   domain.TypeMention,
			Payload: map[string]any{
				"task_id":      p.TaskID,
				"comment_id":   p.CommentID,
				"author_id":    p.AuthorID,
				"body_excerpt": p.BodyExcerpt,
			},
			ProjectID:     uuidPtr(projID),
			SourceEventID: &sourceID,
		})
	}

	// comment_on_my_task for assignee (if not author and not already mentioned)
	if assignee != uuid.Nil && assignee != author && !mentionSet[assignee] {
		sourceID := env.MessageID + ":comment_on_task:" + p.AssigneeID
		result.Notifications = append(result.Notifications, NotificationToCreate{
			UserID: assignee,
			Type:   domain.TypeCommentOnMyTask,
			Payload: map[string]any{
				"task_id":    p.TaskID,
				"comment_id": p.CommentID,
				"author_id":  p.AuthorID,
			},
			ProjectID:     uuidPtr(projID),
			SourceEventID: &sourceID,
		})
	}
	return result, nil
}

// ── Project member handlers ───────────────────────────────────────────────────

type projectMemberAddedHandler struct{}

func (h *projectMemberAddedHandler) Handle(_ context.Context, env Envelope) (*DispatchResult, error) {
	var p struct {
		ProjectID string `json:"project_id"`
		UserID    string `json:"user_id"`
		AddedBy   string `json:"added_by"`
	}
	if err := unmarshal(env.Payload, &p); err != nil {
		return nil, err
	}
	projID := mustUUIDStr(p.ProjectID)
	ch := domain.ProjectChannel(projID)
	frame, _ := ws.EventFrame(string(ch), env.EventType, time.Now(), p)
	result := &DispatchResult{ChannelPushes: []ChannelPush{{Channel: ch, Data: frame}}}

	uid := mustUUIDStr(p.UserID)
	if uid != uuid.Nil {
		sourceID := env.MessageID + ":member_added:" + p.UserID
		result.Notifications = append(result.Notifications, NotificationToCreate{
			UserID:        uid,
			Type:          domain.TypeProjectMemberAdded,
			Payload:       map[string]any{"project_id": p.ProjectID, "added_by": p.AddedBy},
			ProjectID:     uuidPtr(projID),
			SourceEventID: &sourceID,
		})
	}
	return result, nil
}

type projectMemberRemovedHandler struct{}

func (h *projectMemberRemovedHandler) Handle(_ context.Context, env Envelope) (*DispatchResult, error) {
	var p struct {
		ProjectID string `json:"project_id"`
		UserID    string `json:"user_id"`
		RemovedBy string `json:"removed_by"`
	}
	if err := unmarshal(env.Payload, &p); err != nil {
		return nil, err
	}
	projID := mustUUIDStr(p.ProjectID)
	ch := domain.ProjectChannel(projID)
	frame, _ := ws.EventFrame(string(ch), env.EventType, time.Now(), p)
	result := &DispatchResult{ChannelPushes: []ChannelPush{{Channel: ch, Data: frame}}}

	uid := mustUUIDStr(p.UserID)
	if uid != uuid.Nil {
		sourceID := env.MessageID + ":member_removed:" + p.UserID
		result.Notifications = append(result.Notifications, NotificationToCreate{
			UserID:        uid,
			Type:          domain.TypeProjectMemberRemoved,
			Payload:       map[string]any{"project_id": p.ProjectID, "removed_by": p.RemovedBy},
			ProjectID:     uuidPtr(projID),
			SourceEventID: &sourceID,
		})
	}
	return result, nil
}

type projectDeletedHandler struct{}

func (h *projectDeletedHandler) Handle(_ context.Context, env Envelope) (*DispatchResult, error) {
	var p struct {
		ProjectID string `json:"project_id"`
	}
	if err := unmarshal(env.Payload, &p); err != nil {
		return nil, err
	}
	projID := mustUUIDStr(p.ProjectID)
	ch := domain.ProjectChannel(projID)
	frame, _ := ws.EventFrame(string(ch), env.EventType, time.Now(), p)
	return &DispatchResult{ChannelPushes: []ChannelPush{{Channel: ch, Data: frame}}}, nil
}

// ── Invitation handlers ───────────────────────────────────────────────────────

type projectInvitationSentHandler struct{}

func (h *projectInvitationSentHandler) Handle(_ context.Context, env Envelope) (*DispatchResult, error) {
	var p struct {
		InvitationID  string `json:"invitation_id"`
		ProjectID     string `json:"project_id"`
		ProjectName   string `json:"project_name"`
		InviteeID     string `json:"invitee_id"`
		InviteeEmail  string `json:"invitee_email"`
		InvitedBy     string `json:"invited_by"`
		InvitedByEmail string `json:"invited_by_email"`
		Role          string `json:"role"`
		Token         string `json:"token"`
		ExpiresAt     string `json:"expires_at"`
	}
	if err := unmarshal(env.Payload, &p); err != nil {
		return nil, err
	}

	inviteeID := mustUUIDStr(p.InviteeID)
	if inviteeID == uuid.Nil {
		return nil, nil // invitee not yet registered, nothing to notify
	}
	projID := mustUUIDStr(p.ProjectID)
	sourceID := env.MessageID + ":invitation_sent:" + p.InviteeID
	return &DispatchResult{
		Notifications: []NotificationToCreate{{
			UserID: inviteeID,
			Type:   domain.TypeProjectInvitationSent,
			Payload: map[string]any{
				"invitation_id":   p.InvitationID,
				"project_id":      p.ProjectID,
				"project_name":    p.ProjectName,
				"invited_by":      p.InvitedBy,
				"invited_by_email": p.InvitedByEmail,
				"role":            p.Role,
				"token":           p.Token,
				"expires_at":      p.ExpiresAt,
			},
			ProjectID:     uuidPtr(projID),
			SourceEventID: &sourceID,
		}},
	}, nil
}

type projectInvitationAcceptedHandler struct{}

func (h *projectInvitationAcceptedHandler) Handle(_ context.Context, env Envelope) (*DispatchResult, error) {
	var p struct {
		InvitationID string `json:"invitation_id"`
		ProjectID    string `json:"project_id"`
		ProjectName  string `json:"project_name"`
		InviteeID    string `json:"invitee_id"`
		InviteeEmail string `json:"invitee_email"`
		InvitedBy    string `json:"invited_by"`
	}
	if err := unmarshal(env.Payload, &p); err != nil {
		return nil, err
	}

	projID := mustUUIDStr(p.ProjectID)
	// Push to project channel so all connected members see it immediately
	ch := domain.ProjectChannel(projID)
	frame, _ := ws.EventFrame(string(ch), env.EventType, time.Now(), p)
	result := &DispatchResult{ChannelPushes: []ChannelPush{{Channel: ch, Data: frame}}}

	// Persistent notification for the inviter
	inviterID := mustUUIDStr(p.InvitedBy)
	if inviterID != uuid.Nil {
		sourceID := env.MessageID + ":invitation_accepted:" + p.InviteeID
		result.Notifications = append(result.Notifications, NotificationToCreate{
			UserID: inviterID,
			Type:   domain.TypeProjectInvitationAccepted,
			Payload: map[string]any{
				"invitation_id": p.InvitationID,
				"project_id":    p.ProjectID,
				"project_name":  p.ProjectName,
				"invitee_email": p.InviteeEmail,
			},
			ProjectID:     uuidPtr(projID),
			SourceEventID: &sourceID,
		})
	}
	return result, nil
}

type projectInvitationDeclinedHandler struct{}

func (h *projectInvitationDeclinedHandler) Handle(_ context.Context, env Envelope) (*DispatchResult, error) {
	var p struct {
		InvitationID string `json:"invitation_id"`
		ProjectID    string `json:"project_id"`
		ProjectName  string `json:"project_name"`
		InviteeEmail string `json:"invitee_email"`
		InvitedBy    string `json:"invited_by"`
	}
	if err := unmarshal(env.Payload, &p); err != nil {
		return nil, err
	}

	projID := mustUUIDStr(p.ProjectID)
	inviterID := mustUUIDStr(p.InvitedBy)
	if inviterID == uuid.Nil {
		return nil, nil
	}
	sourceID := env.MessageID + ":invitation_declined"
	return &DispatchResult{
		Notifications: []NotificationToCreate{{
			UserID: inviterID,
			Type:   domain.TypeProjectInvitationDeclined,
			Payload: map[string]any{
				"invitation_id": p.InvitationID,
				"project_id":    p.ProjectID,
				"project_name":  p.ProjectName,
				"invitee_email": p.InviteeEmail,
			},
			ProjectID:     uuidPtr(projID),
			SourceEventID: &sourceID,
		}},
	}, nil
}

// ── UUID helpers ──────────────────────────────────────────────────────────────

func mustUUID(v any) uuid.UUID {
	if s, ok := v.(string); ok {
		id, _ := uuid.Parse(s)
		return id
	}
	return uuid.Nil
}

func mustUUIDStr(s string) uuid.UUID {
	id, _ := uuid.Parse(s)
	return id
}

func uuidPtr(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

func marshalPayload(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

var _ = marshalPayload // suppress unused warning
