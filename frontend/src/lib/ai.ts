import { API_BASE, getAuthToken, UNAUTHORIZED_EVENT } from "@/lib/api";

export interface AIConfig {
  baseUrl: string;
  apiKey: string;
  model: string;
  allowWrite: boolean;
  confirmWrites: boolean;
}

export const DEFAULT_AI_CONFIG: AIConfig = {
  baseUrl: "https://api.deepseek.com/v1",
  apiKey: "",
  model: "deepseek-chat",
  allowWrite: false,
  confirmWrites: true,
};

export const AI_PROVIDERS: Array<{ id: string; label: string; baseUrl: string; model: string }> = [
  { id: "deepseek", label: "DeepSeek（默认）", baseUrl: "https://api.deepseek.com/v1", model: "deepseek-chat" },
  { id: "openai", label: "OpenAI", baseUrl: "https://api.openai.com/v1", model: "gpt-4o-mini" },
  { id: "moonshot", label: "Moonshot", baseUrl: "https://api.moonshot.cn/v1", model: "moonshot-v1-8k" },
  { id: "zhipu", label: "智谱 GLM", baseUrl: "https://open.bigmodel.cn/api/paas/v4", model: "glm-4-flash" },
  { id: "ollama", label: "Ollama（本地）", baseUrl: "http://localhost:11434/v1", model: "qwen2.5:7b" },
];

const CONFIG_KEY = "mapserver_ai_config_v1";

export function loadAIConfig(): AIConfig {
  try {
    const raw = localStorage.getItem(CONFIG_KEY);
    if (!raw) return { ...DEFAULT_AI_CONFIG };
    const parsed = JSON.parse(raw) as Partial<AIConfig>;
    return {
      baseUrl: parsed.baseUrl || DEFAULT_AI_CONFIG.baseUrl,
      apiKey: parsed.apiKey || "",
      model: parsed.model || DEFAULT_AI_CONFIG.model,
      allowWrite: parsed.allowWrite ?? false,
      confirmWrites: parsed.confirmWrites ?? true,
    };
  } catch {
    return { ...DEFAULT_AI_CONFIG };
  }
}

export function saveAIConfig(config: AIConfig): void {
  try {
    localStorage.setItem(CONFIG_KEY, JSON.stringify(config));
  } catch {
    /* ignore quota errors */
  }
}

export interface AIChatMessage {
  role: "user" | "assistant";
  content: string;
}

export interface AIMapContext {
  visibleLayers?: string[];
  selectedLayer?: string;
  bounds?: [number, number, number, number];
}

export interface AIStep {
  tool: string;
  summary: string;
  args?: unknown;
}

export interface AIAction {
  type:
    | "fly_to_layer"
    | "fly_to_bounds"
    | "highlight_features"
    | "open_data"
    | "set_layer_visibility";
  layer?: string;
  bounds?: [number, number, number, number];
  featureIds?: number[];
  bbox?: [number, number, number, number];
  query?: string;
  visible?: boolean;
  message?: string;
}

export interface AIResult {
  layer: string;
  layerTitle: string;
  feature: {
    id?: number | string;
    type?: string;
    geometry?: unknown;
    properties?: Record<string, unknown> | null;
  };
  total: number;
}

export interface AIPendingWrite {
  token: string;
  tool: string;
  layer: string;
  summary: string;
  args?: unknown;
}

export interface AIChatResponse {
  message: string;
  steps?: AIStep[];
  actions?: AIAction[];
  results?: AIResult[];
  pendingWrites?: AIPendingWrite[];
  usage?: unknown;
}

/** 发送自然语言请求。API Key 仅随本次请求发送，不写入服务端数据库。 */
export async function aiChat(
  config: AIConfig,
  messages: AIChatMessage[],
  context?: AIMapContext
): Promise<AIChatResponse> {
  const token = getAuthToken();
  const res = await fetch(`${API_BASE}/api/ai/chat`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      "X-AI-API-Key": config.apiKey,
      "X-AI-Base-URL": config.baseUrl,
      "X-AI-Model": config.model,
    },
    body: JSON.stringify({
      messages,
      context,
      allowWrite: config.allowWrite,
      confirmWrites: config.confirmWrites,
    }),
  });
  if (res.status === 401 && typeof window !== "undefined") {
    window.dispatchEvent(new Event(UNAUTHORIZED_EVENT));
  }
  if (!res.ok) {
    let message = `请求失败 (${res.status})`;
    try {
      const body = (await res.json()) as { error?: string };
      if (body?.error) message = body.error;
    } catch {
      /* ignore */
    }
    throw new Error(message);
  }
  return (await res.json()) as AIChatResponse;
}

/** 确认执行 AI 提出的写操作。 */
export async function aiConfirm(token: string): Promise<AIChatResponse> {
  const authToken = getAuthToken();
  const res = await fetch(`${API_BASE}/api/ai/confirm`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      ...(authToken ? { Authorization: `Bearer ${authToken}` } : {}),
    },
    body: JSON.stringify({ token }),
  });
  if (!res.ok) {
    let message = `确认失败 (${res.status})`;
    try {
      const body = (await res.json()) as { error?: string };
      if (body?.error) message = body.error;
    } catch {
      /* ignore */
    }
    throw new Error(message);
  }
  return (await res.json()) as AIChatResponse;
}
