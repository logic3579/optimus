package menu

type MenuNode struct {
	ID             uint64     `json:"id"`
	ParentID       *uint64    `json:"parent_id,omitempty"`
	Code           string     `json:"code"`
	Name           string     `json:"name"`
	Path           string     `json:"path"`
	Component      string     `json:"component"`
	Icon           string     `json:"icon"`
	PermissionCode *string    `json:"permission_code,omitempty"`
	SortOrder      int        `json:"sort_order"`
	Hidden         bool       `json:"hidden"`
	Children       []MenuNode `json:"children,omitempty"`
}

type CreateRequest struct {
	ParentID       *uint64 `json:"parent_id"`
	Code           string  `json:"code"            binding:"required,min=2,max=64"`
	Name           string  `json:"name"            binding:"required,max=128"`
	Path           string  `json:"path"            binding:"max=255"`
	Component      string  `json:"component"       binding:"max=255"`
	Icon           string  `json:"icon"            binding:"max=64"`
	PermissionCode *string `json:"permission_code" binding:"omitempty,max=128"`
	SortOrder      int     `json:"sort_order"`
	Hidden         bool    `json:"hidden"`
}

type UpdateRequest struct {
	ParentID       *uint64 `json:"parent_id"`
	Name           *string `json:"name"            binding:"omitempty,max=128"`
	Path           *string `json:"path"            binding:"omitempty,max=255"`
	Component      *string `json:"component"       binding:"omitempty,max=255"`
	Icon           *string `json:"icon"            binding:"omitempty,max=64"`
	PermissionCode *string `json:"permission_code" binding:"omitempty,max=128"`
	SortOrder      *int    `json:"sort_order"`
	Hidden         *bool   `json:"hidden"`
}
