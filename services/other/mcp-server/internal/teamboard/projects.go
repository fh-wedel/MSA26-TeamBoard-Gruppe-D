package teamboard

import "context"

// ListProjects returns all projects the token's owning user is a member of.
func (c *Client) ListProjects(ctx context.Context) ([]Project, error) {
	var projects []Project
	if _, err := c.doList(ctx, "GET", "/projects", &projects); err != nil {
		return nil, err
	}
	return projects, nil
}
