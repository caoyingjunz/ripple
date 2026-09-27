export type User = {
  id: string;
  username: string;
  display_name: string;
  avatar_color: string;
};

export type Conversation = {
  id: string;
  peer: User;
  last_message?: string;
  last_message_at?: number;
  created_at: number;
};

export type Message = {
  id: string;
  conversation_id: string;
  sender_id: string;
  type: "text" | "image";
  body: string;
  image_url?: string;
  created_at: number;
};

export type Bottle = {
  id: string;
  content: string;
  status: string;
  created_at: number;
  thread_id?: string;
  role?: string;
};

export type BottleMessage = {
  id: string;
  thread_id: string;
  alias: string;
  mine: boolean;
  body: string;
  created_at: number;
};

export type BottleThread = {
  id: string;
  bottle_id: string;
  bottle_content: string;
  my_alias: string;
  round_count: number;
  max_rounds: number;
  revealed: boolean;
  conversation_id?: string;
  closed: boolean;
  messages: BottleMessage[];
  created_at: number;
};
