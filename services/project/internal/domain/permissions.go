package domain

var viewerPerms = []string{
	"project:read",
	"board:read",
	"task:read",
	"comment:read",
	"document:read",
	"member:read",
}

var editorPerms = append(viewerPerms,
	"project:update",
	"board:create",
	"board:update",
	"task:create",
	"task:update",
	"task:delete",
	"comment:create",
	"document:create",
	"document:update",
	"document:delete",
)

var ownerPerms = append(editorPerms,
	"project:delete",
	"board:delete",
	"member:invite",
	"member:update",
	"member:remove",
	"webhook:manage",
)

// PermissionsForRole returns the full expanded permission set for a role.
func PermissionsForRole(r Role) []string {
	switch r {
	case RoleViewer:
		out := make([]string, len(viewerPerms))
		copy(out, viewerPerms)
		return out
	case RoleEditor:
		out := make([]string, len(editorPerms))
		copy(out, editorPerms)
		return out
	case RoleOwner:
		out := make([]string, len(ownerPerms))
		copy(out, ownerPerms)
		return out
	default:
		return nil
	}
}

// HasPermission returns true when the role includes the given permission.
func HasPermission(r Role, perm string) bool {
	for _, p := range PermissionsForRole(r) {
		if p == perm {
			return true
		}
	}
	return false
}
