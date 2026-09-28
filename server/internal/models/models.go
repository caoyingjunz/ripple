package models

type User struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	DisplayName string  `json:"display_name"`
	AvatarColor string  `json:"avatar_color"`
	AvatarURL   *string `json:"avatar_url"`
	Signature   string  `json:"signature"`
	Status      string  `json:"status"` // online | offline
}

type Message struct {
	ID             string  `json:"id"`
	ConversationID string  `json:"conversation_id"`
	Seq            int64   `json:"seq"`
	SenderID       string  `json:"sender_id"`
	Type           string  `json:"type"` // text | image | system
	Body           string  `json:"body"`
	ImageURL       *string `json:"image_url"`
	RevokedAt      *int64  `json:"revoked_at"`
	CreatedAt      int64   `json:"created_at"`
}

type ConversationSummary struct {
	ID                 string  `json:"id"`
	Type               string  `json:"type"` // single | group
	Name               *string `json:"name"`
	Peer               *User   `json:"peer"` // single 型对端
	OwnerID            *string `json:"owner_id"`
	MemberCount        int     `json:"member_count"`
	Unread             int64   `json:"unread"`
	LastSeq            int64   `json:"last_seq"`
	LastReadSeq        int64   `json:"last_read_seq"`
	LastMessagePreview *string `json:"last_message_preview"`
	LastMessageAt      *int64  `json:"last_message_at"`
	Pinned             bool    `json:"pinned"`
	Muted              bool    `json:"muted"`
	CreatedAt          int64   `json:"created_at"`
}

type MemberEntry struct {
	User     User    `json:"user"`
	Role     string  `json:"role"` // owner | member
	Alias    *string `json:"alias"`
	JoinedAt int64   `json:"joined_at"`
}

type FriendRequest struct {
	ID        string `json:"id"`
	User      User   `json:"user"`
	CreatedAt int64  `json:"created_at"`
}

type SearchEntry struct {
	User     User   `json:"user"`
	Relation string `json:"relation"` // none | pending_out | pending_in | accepted | blocked
}

type FriendsView struct {
	Friends    []User          `json:"friends"`
	PendingIn  []FriendRequest `json:"pending_in"`
	PendingOut []FriendRequest `json:"pending_out"`
	Blocked    []User          `json:"blocked"`
}

type Bottle struct {
	ID             string  `json:"id"`
	Content        string  `json:"content"`
	Status         string  `json:"status"` // floating | picked | closed
	CreatedAt      int64   `json:"created_at"`
	ThreadID       *string `json:"thread_id"`
	Role           string  `json:"role"` // thrower | picker
	ConversationID *string `json:"conversation_id"`
	Unread         int64   `json:"unread"`
}

type BottleMessageView struct {
	ID              string `json:"id"`
	ConversationID  string `json:"conversation_id"`
	Seq             int64  `json:"seq"`
	Alias           string `json:"alias"`
	Mine            bool   `json:"mine"`
	Type            string `json:"type"`
	Body            string `json:"body"`
	CreatedAt       int64  `json:"created_at"`
	// 匿名投影：绝不携带 sender_id
}

type BottleThreadView struct {
	ID              string              `json:"id"`
	BottleID        string              `json:"bottle_id"`
	BottleContent   string              `json:"bottle_content"`
	MyAlias         string              `json:"my_alias"`
	RoundCount      int                 `json:"round_count"`
	MaxRounds       int                 `json:"max_rounds"`
	Revealed        bool                `json:"revealed"`
	Closed          bool                `json:"closed"`
	ConversationID  *string             `json:"conversation_id"`
	CreatedAt       int64               `json:"created_at"`
	Messages        []BottleMessageView `json:"messages"`
}
