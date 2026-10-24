import { useEffect, useMemo, useRef, useState } from "react";
import {
  Bot,
  Check,
  KeyRound,
  Loader2,
  Send,
  Settings,
  Sparkles,
  Trash2,
  User,
  Wrench,
  X,
} from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import {
  AI_PROVIDERS,
  DEFAULT_AI_CONFIG,
  aiChat,
  aiConfirm,
  loadAIConfig,
  saveAIConfig,
  type AIAction,
  type AIChatMessage,
  type AIMapContext,
  type AIChatResponse,
  type AIConfig,
} from "@/lib/ai";

const EXAMPLES = [
  "有哪些图层？分别有多少要素？",
  "查找人口超过 1000 万的城市，并定位到第一个",
  "统计国家图层的人口分布",
  "把城市图层缩放到地图上",
  "查询中国范围内的城市，并打开数据表",
];

export function AiAssistant({
  open,
  onOpenChange,
  context,
  onAction,
  onFocusFeature,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  context: AIMapContext;
  onAction?: (action: AIAction) => void;
  onFocusFeature?: (layer: string, fid: number | string) => void;
}) {
  const [config, setConfig] = useState<AIConfig>(() => loadAIConfig());
  const [draft, setDraft] = useState<AIConfig>(config);
  const [settings, setSettings] = useState(false);
  const [messages, setMessages] = useState<AIChatMessage[]>([]);
  const [input, setInput] = useState("");
  const [sending, setSending] = useState(false);
  const [error, setError] = useState("");
  const [last, setLast] = useState<AIChatResponse | null>(null);
  const [confirming, setConfirming] = useState("");
  const bottomRef = useRef<HTMLDivElement>(null);

  const providerId = useMemo(() => {
    return AI_PROVIDERS.find((p) => p.baseUrl === draft.baseUrl)?.id ?? "custom";
  }, [draft.baseUrl]);

  useEffect(() => {
    if (!open) return;
    setDraft(config);
    if (!config.apiKey) setSettings(true);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages, sending, last]);

  const saveSettings = () => {
    saveAIConfig(draft);
    setConfig(draft);
    setSettings(false);
    toast.success("AI 配置已保存在本浏览器");
  };

  const send = async (text?: string) => {
    const content = (text ?? input).trim();
    if (!content || sending) return;
    if (!config.apiKey) {
      setSettings(true);
      toast.error("请先填写你的模型 API Key（BYOK）");
      return;
    }
    const next: AIChatMessage[] = [...messages, { role: "user", content }];
    setMessages(next);
    setInput("");
    setError("");
    setLast(null);
    setSending(true);
    try {
      const res = await aiChat(config, next, context);
      setMessages([...next, { role: "assistant", content: res.message }]);
      setLast(res);
      for (const action of res.actions ?? []) onAction?.(action);
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setSending(false);
    }
  };

  const clear = () => {
    setMessages([]);
    setLast(null);
    setError("");
  };

  const confirmWrite = async (token: string, summary: string) => {
    setConfirming(token);
    try {
      const res = await aiConfirm(token);
      setLast((prev) =>
        prev
          ? { ...prev, pendingWrites: prev.pendingWrites?.filter((x) => x.token !== token) }
          : prev
      );
      for (const action of res.actions ?? []) onAction?.(action);
      setMessages((prev) => [...prev, { role: "assistant", content: `已执行：${summary}` }]);
      toast.success("已执行 AI 写操作");
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setConfirming("");
    }
  };

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="flex w-full flex-col p-0 sm:max-w-lg">
        <SheetHeader className="border-b p-4 pr-12">
          <SheetTitle className="flex items-center gap-2 text-base">
            <span className="flex h-7 w-7 items-center justify-center rounded-md bg-primary text-primary-foreground">
              <Sparkles className="h-3.5 w-3.5" />
            </span>
            自然语言地图助手
            <Badge variant="outline" className="ml-1 text-[10px]">
              BYOK · ADK 风格工具调用
            </Badge>
          </SheetTitle>
          <div className="flex items-center gap-2 pt-1 text-[11px] text-muted-foreground">
            <span className="truncate">
              模型：{config.model} · {config.baseUrl}
            </span>
            <Button
              variant="ghost"
              size="sm"
              className="ml-auto h-7"
              onClick={() => setSettings((v) => !v)}
            >
              <Settings className="h-3.5 w-3.5" /> 设置
            </Button>
            {messages.length > 0 && (
              <Button variant="ghost" size="sm" className="h-7" onClick={clear}>
                <Trash2 className="h-3.5 w-3.5" /> 清空
              </Button>
            )}
          </div>
        </SheetHeader>

        {settings ? (
          <div className="flex-1 space-y-4 overflow-y-auto p-4">
            <div className="rounded-md border bg-muted/40 p-3 text-[11px] leading-relaxed text-muted-foreground">
              <KeyRound className="mb-1 inline h-3.5 w-3.5" /> BYOK（Bring Your Own Key）：
              密钥只保存在本浏览器的 localStorage，随每次请求发送到后端代理，不会写入 MapServer
              数据库，也不会出现在日志中。默认使用 DeepSeek。
            </div>
            <div className="grid gap-1.5">
              <Label>服务商预设</Label>
              <Select
                value={providerId}
                onValueChange={(v) => {
                  const p = AI_PROVIDERS.find((x) => x.id === v);
                  if (p) setDraft((d) => ({ ...d, baseUrl: p.baseUrl, model: p.model }));
                }}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {AI_PROVIDERS.map((p) => (
                    <SelectItem key={p.id} value={p.id}>
                      {p.label}
                    </SelectItem>
                  ))}
                  <SelectItem value="custom">自定义（OpenAI 兼容）</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-1.5">
              <Label>Base URL</Label>
              <Input
                value={draft.baseUrl}
                onChange={(e) => setDraft((d) => ({ ...d, baseUrl: e.target.value }))}
                placeholder="https://api.deepseek.com/v1"
              />
            </div>
            <div className="grid gap-1.5">
              <Label>模型</Label>
              <Input
                value={draft.model}
                onChange={(e) => setDraft((d) => ({ ...d, model: e.target.value }))}
                placeholder="deepseek-chat"
              />
            </div>
            <div className="grid gap-1.5">
              <Label>API Key</Label>
              <Input
                type="password"
                value={draft.apiKey}
                onChange={(e) => setDraft((d) => ({ ...d, apiKey: e.target.value }))}
                placeholder="sk-..."
                autoComplete="off"
              />
            </div>
            <label className="flex items-start justify-between gap-3 rounded-md border p-3 text-sm">
              <span>
                允许写操作
                <span className="mt-0.5 block text-[11px] text-muted-foreground">
                  AI 可调用 update_features / delete_features。仅在你信任当前模型与提示词时开启。
                </span>
              </span>
              <Switch
                checked={draft.allowWrite}
                onCheckedChange={(v) => setDraft((d) => ({ ...d, allowWrite: v }))}
              />
            </label>
            {draft.allowWrite && (
              <label className="flex items-start justify-between gap-3 rounded-md border p-3 text-sm">
                <span>
                  写操作二次确认
                  <span className="mt-0.5 block text-[11px] text-muted-foreground">
                    默认开启：AI 只生成待确认令牌，必须由你点击「确认执行」才会修改数据。
                  </span>
                </span>
                <Switch
                  checked={draft.confirmWrites}
                  onCheckedChange={(v) => setDraft((d) => ({ ...d, confirmWrites: v }))}
                />
              </label>
            )}
            <div className="flex gap-2">
              <Button className="flex-1" onClick={saveSettings}>
                <Check className="h-4 w-4" /> 保存配置
              </Button>
              <Button
                variant="outline"
                onClick={() => {
                  setDraft({ ...DEFAULT_AI_CONFIG, apiKey: draft.apiKey });
                  setConfig((c) => ({ ...DEFAULT_AI_CONFIG, apiKey: c.apiKey }));
                  saveAIConfig({ ...DEFAULT_AI_CONFIG, apiKey: draft.apiKey });
                  toast.success("已恢复 DeepSeek 默认配置");
                }}
              >
                恢复默认
              </Button>
            </div>
          </div>
        ) : (
          <>
            <ScrollArea className="min-h-0 flex-1">
              <div className="space-y-3 p-4">
                {messages.length === 0 && (
                  <div className="space-y-3">
                    <div className="rounded-lg border bg-muted/30 p-3 text-sm">
                      <div className="mb-1 flex items-center gap-2 font-medium">
                        <Bot className="h-4 w-4" /> 你可以直接说：
                      </div>
                      <p className="text-xs leading-relaxed text-muted-foreground">
                        我会先了解图层与字段，再查询数据、统计数据、定位地图或打开数据表。写操作默认关闭。
                      </p>
                    </div>
                    <div className="grid gap-2">
                      {EXAMPLES.map((ex) => (
                        <button
                          key={ex}
                          className="rounded-md border px-3 py-2 text-left text-sm transition hover:bg-accent"
                          onClick={() => void send(ex)}
                        >
                          <Sparkles className="mr-1 inline h-3.5 w-3.5 text-primary" />
                          {ex}
                        </button>
                      ))}
                    </div>
                  </div>
                )}

                {messages.map((m, i) => (
                  <div
                    key={i}
                    className={`flex gap-2 ${m.role === "user" ? "justify-end" : "justify-start"}`}
                  >
                    {m.role === "assistant" && (
                      <span className="mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-primary/10 text-primary">
                        <Bot className="h-3.5 w-3.5" />
                      </span>
                    )}
                    <div
                      className={`max-w-[85%] whitespace-pre-wrap rounded-lg px-3 py-2 text-sm ${
                        m.role === "user"
                          ? "bg-primary text-primary-foreground"
                          : "bg-muted"
                      }`}
                    >
                      {m.content}
                    </div>
                    {m.role === "user" && (
                      <span className="mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-muted">
                        <User className="h-3.5 w-3.5" />
                      </span>
                    )}
                  </div>
                ))}

                {sending && (
                  <div className="flex items-center gap-2 text-sm text-muted-foreground">
                    <Loader2 className="h-4 w-4 animate-spin" /> 正在理解并调用地图工具…
                  </div>
                )}

                {last?.steps && last.steps.length > 0 && (
                  <div className="rounded-md border bg-muted/30 p-3">
                    <div className="mb-1 flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
                      <Wrench className="h-3.5 w-3.5" /> 执行过程
                    </div>
                    <div className="space-y-1">
                      {last.steps.map((s, i) => (
                        <div key={i} className="flex items-start gap-2 text-[11px]">
                          <Badge variant="outline" className="shrink-0 font-mono text-[10px]">
                            {s.tool}
                          </Badge>
                          <span className="text-muted-foreground">{s.summary}</span>
                        </div>
                      ))}
                    </div>
                  </div>
                )}

                {last?.results && last.results.length > 0 && (
                  <div className="space-y-1 rounded-md border p-2">
                    <div className="px-1 text-xs font-medium text-muted-foreground">
                      查询结果（点击定位）
                    </div>
                    <div className="max-h-64 space-y-1 overflow-y-auto">
                      {last.results.slice(0, 30).map((r, i) => {
                        const props = r.feature.properties ?? {};
                        const label =
                          String(props.name ?? props.title ?? props.label ?? "") ||
                          `#${r.feature.id ?? i + 1}`;
                        return (
                          <button
                            key={`${r.layer}-${r.feature.id}-${i}`}
                            className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-sm hover:bg-accent"
                            onClick={() => {
                              if (r.feature.id !== undefined) {
                                onFocusFeature?.(r.layer, r.feature.id);
                              }
                            }}
                          >
                            <Badge variant="secondary" className="shrink-0 text-[10px]">
                              {r.layerTitle || r.layer}
                            </Badge>
                            <span className="min-w-0 flex-1 truncate">{label}</span>
                            <span className="text-[10px] text-muted-foreground">
                              #{String(r.feature.id ?? "")}
                            </span>
                          </button>
                        );
                      })}
                    </div>
                  </div>
                )}

                {last?.pendingWrites && last.pendingWrites.length > 0 && (
                  <div className="space-y-2 rounded-md border border-amber-300 bg-amber-50 p-3 dark:bg-amber-950/30">
                    <div className="text-xs font-medium text-amber-800 dark:text-amber-300">
                      待确认的写操作
                    </div>
                    {last.pendingWrites.map((p) => (
                      <div key={p.token} className="flex flex-wrap items-center gap-2 text-xs">
                        <Badge variant="outline" className="font-mono text-[10px]">
                          {p.tool}
                        </Badge>
                        <span className="min-w-0 flex-1">{p.summary}</span>
                        <Button
                          size="sm"
                          className="h-7"
                          disabled={confirming === p.token}
                          onClick={() => void confirmWrite(p.token, p.summary)}
                        >
                          {confirming === p.token ? (
                            <Loader2 className="h-3.5 w-3.5 animate-spin" />
                          ) : (
                            <Check className="h-3.5 w-3.5" />
                          )}
                          确认执行
                        </Button>
                        <Button
                          size="sm"
                          variant="ghost"
                          className="h-7"
                          onClick={() =>
                            setLast((prev) =>
                              prev
                                ? {
                                    ...prev,
                                    pendingWrites: prev.pendingWrites?.filter(
                                      (x) => x.token !== p.token
                                    ),
                                  }
                                : prev
                            )
                          }
                        >
                          忽略
                        </Button>
                      </div>
                    ))}
                  </div>
                )}

                {error && (
                  <div className="flex items-start justify-between gap-2 rounded-md border border-destructive/40 bg-destructive/10 p-2 text-xs text-destructive">
                    <span>{error}</span>
                    <button onClick={() => setError("")} aria-label="关闭">
                      <X className="h-3.5 w-3.5" />
                    </button>
                  </div>
                )}
                <div ref={bottomRef} />
              </div>
            </ScrollArea>

            <div className="border-t p-3">
              <div className="flex items-end gap-2">
                <Textarea
                  rows={2}
                  className="min-h-[44px] resize-none"
                  placeholder="例如：找出人口最多的 5 个城市并定位…"
                  value={input}
                  onChange={(e) => setInput(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" && !e.shiftKey) {
                      e.preventDefault();
                      void send();
                    }
                  }}
                  disabled={sending}
                />
                <Button
                  size="icon"
                  className="h-11 w-11 shrink-0"
                  onClick={() => void send()}
                  disabled={sending || !input.trim()}
                >
                  {sending ? (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  ) : (
                    <Send className="h-4 w-4" />
                  )}
                </Button>
              </div>
              <p className="mt-1.5 text-[10px] text-muted-foreground">
                默认 DeepSeek · 密钥仅保存在本浏览器 · 写操作
                {config.allowWrite
                  ? config.confirmWrites
                    ? "已开启（需二次确认）"
                    : "已开启（直接执行）"
                  : "已关闭"}
              </p>
            </div>
          </>
        )}
      </SheetContent>
    </Sheet>
  );
}
