package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/teamboard/services/other/mcp-server/internal/teamboard"
)

func registerProjectTools(server *mcp.Server, client *teamboard.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_projects",
		Description: "List all TeamBoard projects the authenticated user is a member of.",
	}, listProjects(client))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_project",
		Description: "Create a new TeamBoard project. The authenticated user becomes its owner.",
	}, createProject(client))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_project",
		Description: "Rename a project and/or change its description. Only the fields you pass are changed.",
	}, updateProject(client))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_project",
		Description: "Permanently delete a project including all of its boards and tasks. Irreversible — confirm with the user before calling.",
	}, deleteProject(client))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_project_members",
		Description: "List a project's members with their user IDs, emails and roles — e.g. to find an assignee's user_id for assign_task.",
	}, listProjectMembers(client))
}

// ── list_projects ────────────────────────────────────────────────────────────

type listProjectsInput struct{}

func listProjects(client *teamboard.Client) mcp.ToolHandlerFor[listProjectsInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, _ listProjectsInput) (*mcp.CallToolResult, any, error) {
		projects, err := client.ListProjects(ctx)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(projects)
	}
}

// ── create_project ───────────────────────────────────────────────────────────

type createProjectInput struct {
	Name        string `json:"name" jsonschema:"Project name"`
	Description string `json:"description,omitempty" jsonschema:"Optional project description"`
}

func createProject(client *teamboard.Client) mcp.ToolHandlerFor[createProjectInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in createProjectInput) (*mcp.CallToolResult, any, error) {
		project, err := client.CreateProject(ctx, in.Name, in.Description)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(project)
	}
}

// ── update_project ───────────────────────────────────────────────────────────

type updateProjectInput struct {
	ProjectID   string  `json:"project_id"`
	Name        *string `json:"name,omitempty" jsonschema:"New project name; omit to keep"`
	Description *string `json:"description,omitempty" jsonschema:"New description; omit to keep"`
}

func updateProject(client *teamboard.Client) mcp.ToolHandlerFor[updateProjectInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in updateProjectInput) (*mcp.CallToolResult, any, error) {
		patch := map[string]any{}
		if in.Name != nil {
			patch["name"] = *in.Name
		}
		if in.Description != nil {
			patch["description"] = *in.Description
		}
		if len(patch) == 0 {
			return nil, nil, errNothingToUpdate
		}
		project, err := client.UpdateProject(ctx, in.ProjectID, patch)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(project)
	}
}

// ── delete_project ───────────────────────────────────────────────────────────

type deleteProjectInput struct {
	ProjectID string `json:"project_id" jsonschema:"The project to delete permanently"`
}

func deleteProject(client *teamboard.Client) mcp.ToolHandlerFor[deleteProjectInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in deleteProjectInput) (*mcp.CallToolResult, any, error) {
		if err := client.DeleteProject(ctx, in.ProjectID); err != nil {
			return nil, nil, err
		}
		return jsonResult(map[string]string{"status": "deleted", "project_id": in.ProjectID})
	}
}

// ── list_project_members ─────────────────────────────────────────────────────

type listProjectMembersInput struct {
	ProjectID string `json:"project_id"`
}

func listProjectMembers(client *teamboard.Client) mcp.ToolHandlerFor[listProjectMembersInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in listProjectMembersInput) (*mcp.CallToolResult, any, error) {
		members, err := client.ListMembers(ctx, in.ProjectID)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(members)
	}
}
