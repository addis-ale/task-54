package domain

type Tag struct {
	ID      int64  `json:"id"`
	TagType string `json:"tag_type"`
	Name    string `json:"name"`
}

const (
	TagTypeFocus     = "focus"
	TagTypeEquipment = "equipment"
	TagTypeGeneral   = "general"
)

func IsValidTagType(tagType string) bool {
	switch tagType {
	case TagTypeFocus, TagTypeEquipment, TagTypeGeneral:
		return true
	default:
		return false
	}
}
