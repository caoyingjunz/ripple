/** REST / WS 契约类型（唯一标准：docs/implementation-spec.md §3-§4）。 */

export type User = {
  id: string;
  username: string;
  display_name: string;
  avatar_color: string;
  avatar_url: string | null;
  signature: string;
  status: "online" | "offline";
};

export type Relation = "none" | "pending_out" | "pending_in" | "accepted" | "blocked";

export type UserSearchItem = { user: User; relation: Relation };

export type FriendRequest = { id: string; user: User; created_at: number };

export type FriendsOverview = {
  friends: User[];
  pending_in: FriendRequest[];
  pending_out: FriendRequest[];
  blocked: User[];
};

export type ConversationSummary = {
  id: string;
  type: "single" | "group";
  name: string | null;
  peer: User | null;
  owner_id: string | null;
  member_count: number;
  unread: number;
  last_seq: number;
  last_read_seq: number;
  last_message_preview: string | null;
  last_message_at: number | null;
  pinned: boolean;
  muted: boolean;
  created_at: number;
};

export type ConversationMember = {
  user: User;
  role: "owner" | "member";
  alias: string | null;
  joined_at: number;
};

export type ConversationDetail = {
  conversation: ConversationSummary;
  members: ConversationMember[];
};

export type MessageType = "text" | "image" | "system";

export type Message = {
  id: string;
  conversation_id: string;
  seq: number;
  sender_id: string;
  type: MessageType;
  body: string;
  image_url: string | null;
  revoked_at: number | null;
  created_at: number;
};

export type Bottle = {
  id: string;
  thrower_id: string;
  content: string;
  status: "floating" | "picked" | "closed";
  picker_id: string | null;
  created_at: number;
  picked_at: number | null;
  conversation_id?: string | null;
  unread?: number;
  thread_id?: string | null;
};

export type BottleMessageView = {
  id: string;
  conversation_id: string;
  seq: number;
  alias: string;
  mine: boolean;
  type: "text";
  body: string;
  created_at: number;
};

export type BottleThreadView = {
  id: string;
  bottle_id: string;
  bottle_content: string;
  my_alias: string;
  round_count: number;
  max_rounds: number;
  revealed: boolean;
  closed: boolean;
  conversation_id: string | null;
  created_at: number;
  messages: BottleMessageView[];
};
