package models

type User struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AvatarColor string `json:"avatar_color"`
}

type Conversation struct {
	ID            string  `json:"id"`
	Peer          User    `json:"peer"`
	LastMessage   *string `json:"last_message,omitempty"`
	LastMessageAt *int64  `json:"last_message_at,omitempty"`
	CreatedAt     int64   `json:"created_at"`
}

type Message struct {
	ID             string  `json:"id"`
	ConversationID string  `json:"conversation_id"`
	SenderID       string  `json:"sender_id"`
	Type           string  `json:"type"` // text | image
	Body           string  `json:"body"`
	ImageURL       *string `json:"image_url,omitempty"`
	CreatedAt      int64   `json:"created_at"`
}

type Bottle struct {
	ID        string  `json:"id"`
	Content   string  `json:"content"`
	Status    string  `json:"status"`
	CreatedAt int64   `json:"created_at"`
	ThreadID  *string `json:"thread_id,omitempty"`
	Role      string  `json:"role,omitempty"` // thrower | picker
}

type BottleMessage struct {
	ID        string `json:"id"`
	ThreadID  string `json:"thread_id"`
	Alias     string `json:"alias"`
	Mine      bool   `json:"mine"`
	Body      string `json:"body"`
	CreatedAt int64  `json:"created_at"`
}

type BottleThread struct {
	ID             string          `json:"id"`
	BottleID       string          `json:"bottle_id"`
	BottleContent  string          `json:"bottle_content"`
	MyAlias        string          `json:"my_alias"`
	RoundCount     int             `json:"round_count"`
	MaxRounds      int             `json:"max_rounds"`
	Revealed       bool            `json:"revealed"`
	ConversationID *string         `json:"conversation_id,omitempty"`
	Closed         bool            `json:"closed"`
	Messages       []BottleMessage `json:"messages"`
	CreatedAt      int64           `json:"created_at"`
}
