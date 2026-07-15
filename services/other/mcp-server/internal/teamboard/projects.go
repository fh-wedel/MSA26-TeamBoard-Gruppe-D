package teamboard

import (
	"context"
	"fmt"
)

// ListProjects returns all projects the token's owning user is a member of.
func (c *Client) ListProjects(ctx context.Context) ([]Project, error) {
	var projects []Project
	if _, err := c.doList(ctx, "GET", "/projects", &projects); err != nil {
		return nil, err
	}
	return projects, nil
}

// CreateProject creates a project; the token's user becomes its owner.
func (c *Client) CreateProject(ctx context.Context, name, description string) (*Project, error) {
	var project Project
	if err := c.doItem(ctx, "POST", "/projects", map[string]string{"name": name, "description": description}, &project); err != nil {
		return nil, err
	}
	return &project, nil
}

// UpdateProject applies a merge patch (name and/or description) to a project.
func (c *Client) UpdateProject(ctx context.Context, projectID string, patch map[string]any) (*Project, error) {
	var project Project
	if err := c.doItem(ctx, "PATCH", fmt.Sprintf("/projects/%s", projectID), patch, &project); err != nil {
		return nil, err
	}
	return &project, nil
}

// DeleteProject permanently deletes a project with all its boards and tasks.
func (c *Client) DeleteProject(ctx context.Context, projectID string) error {
	return c.doItem(ctx, "DELETE", fmt.Sprintf("/projects/%s", projectID), nil, nil)
}

// ListMembers returns a project's members (user IDs, emails, roles).
func (c *Client) ListMembers(ctx context.Context, projectID string) ([]Member, error) {
	var members []Member
	if _, err := c.doList(ctx, "GET", fmt.Sprintf("/projects/%s/members", projectID), &members); err != nil {
		return nil, err
	}
	return members, nil
}
