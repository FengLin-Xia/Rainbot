# Go Agent Runtime 双模型风格层技术方案

## 1. 文档目标

本文档用于定义一个 **以 Go 为核心运行时、兼容大模型推理与小模型风格层的 Agent 系统方案**。项目的首要目标不是复刻 OpenClaw 的全部功能，而是针对当前使用体验中的核心问题做重构：

1. **运行时链路更稳**：网络连接、流式输出、工具执行、状态管理不能再出现明显卡顿或黑箱感；
2. **预留小模型风格层接口**：允许后续将本地小模型/LoRA 风格改写模块稳定嵌入，而不是事后硬塞；
3. **保留扩展性**：后续可逐步接入技能系统、记忆系统、多 Provider 支持，但第一阶段不追求大而全；
4. **解决“AI 客服假笑”问题**：让回答先有判断，再谈风格，而不是默认输出虚空安慰和过度礼貌。

---

## 2. 背景与问题定义

### 2.1 当前问题来源

现有 OpenClaw 类框架在概念上覆盖很广，但在实际使用中容易出现以下问题：

- 长连接与流式链路不够顺滑，响应体感发闷；
- 技能、记忆、Prompt、人设常常耦合在一起，出了问题难定位；
- 运行时状态不可观测，用户和开发者都难判断“当前到底卡在模型、工具、网络还是框架本身”；
- 风格/人格大多基于 Prompt 软约束，长对话、技能调用或复杂上下文下容易漂移；
- 大模型回答策略默认偏向礼貌、安抚、安全，容易产生“虚空安慰先于问题判断”的体验偏差。

### 2.2 本项目要解决的不是“做一个更像人的 AI”

本项目更具体的目标是：

> **构建一个更顺手的 Agent runtime，并在架构层面为“受控风格后处理”预留稳定插口。**

换句话说，这不是先做人格宇宙，而是先把底盘换掉：

- **先解决 runtime 不顺手的问题**；
- **再解决风格表达与主任务逻辑耦合的问题**。

---

## 3. 总体设计原则

### 3.1 第一原则：先有稳定 runtime，再谈人格表达

小模型风格层很重要，但它不是第一根柱子。若 runtime 本身仍然卡顿、阻塞、状态不透明，那么再多风格层设计也只是贴功能。

### 3.2 第二原则：内容生成与风格表达解耦

系统回答分为两层：

- **上游大模型层**：负责推理、工具调用、知识整合、结果归纳；
- **下游风格层**：负责受控改写输出语气，不重新做主任务决策。

### 3.3 第三原则：先保留接口，再追求完美训练

第一阶段不要求小模型风格层已经完成 LoRA 微调，但要求 runtime **从第一天起就支持 style hook**，避免后续架构返工。

### 3.4 第四原则：明确可观测性优先级

本项目不是只追求“感觉更快”，而是要明确记录：

- 首 token 延迟
- 工具调用耗时
- 风格层耗时
- 总 turn 完成耗时
- 错误与 fallback 原因

---

## 4. 系统目标与非目标

### 4.1 第一阶段目标（v0.1）

构建一个最小但可用的 Go Agent runtime，满足：

- 单会话 Agent 流程可运行；
- 支持至少一个大模型 Provider；
- 支持流式输出；
- 支持至少一个 Tool；
- 支持结构化中间态；
- 支持一个可插拔 Style Processor 接口；
- 具备基础日志与链路观测。

### 4.2 非目标

第一阶段**不追求**以下内容完整上线：

- 完整复刻 OpenClaw channel 体系；
- 多 Agent 协作复杂图编排；
- 全量长期记忆系统；
- 成熟 LoRA 训练闭环；
- 全面 MCP / A2A / Browser / Dashboard 一次性到位。

---

## 5. 总体架构

### 5.1 核心处理链路

```text
User Input
-> Session / Router
-> Prompt Builder + Context Loader
-> Big Model (reasoning / tool decisions)
-> Tool Execution (optional)
-> Structured Response Object
-> Style Processor (small model / rewrite layer)
-> Stream Output / Final Output
```

### 5.2 关键分层

1. **Transport Layer**：HTTP / WebSocket / SSE 连接层；
2. **Runtime Layer**：turn 调度、超时控制、流式回传、错误处理；
3. **LLM Layer**：大模型 Provider 与小模型 Provider 抽象；
4. **Tool Layer**：工具注册、执行、回传；
5. **Response Layer**：中间结构对象与最终输出；
6. **Style Layer**：受控风格处理；
7. **Observability Layer**：日志、trace、性能指标。

---

## 6. 为什么选 Go

Go 在本项目里不是为了“追热点”，而是因为它在以下方面更适合做 **常驻、低延迟、可部署的 Agent runtime**：

- 原生并发模型适合处理流式输出、工具调用和超时控制；
- HTTP / WebSocket / SSE 服务成熟稳定；
- 单二进制部署简单，不容易被环境依赖拖死；
- 对长期运行服务、日志链路、监控接入更友好；
- 比 Node 风格胶水脚本更适合写清楚 runtime 边界。

注意：

> Go 不是自动让 Agent 变快的魔法，真正决定体验的还是链路设计、阻塞点管理和可观测性。

---

## 7. 核心模块设计

### 7.1 Runtime 调度层

负责：

- 接收用户输入；
- 维护 turn 生命周期；
- 控制模型调用顺序；
- 协调 Tool 与 Style Layer；
- 统一错误处理和 fallback；
- 向前端进行流式回传。

关键要求：

- turn 内状态清晰可追踪；
- 支持取消、超时、中断与重试；
- 避免所有逻辑堆在单条串行黑箱链路中。

### 7.2 Provider 抽象层

建议最少支持：

- OpenAI-compatible API
- Anthropic（可选）
- Ollama / 本地模型

建议接口：

```go
// Big model and small model can both implement this.
type ModelClient interface {
    Generate(ctx context.Context, req GenerateRequest) (GenerateResponse, error)
    Stream(ctx context.Context, req GenerateRequest) (<-chan StreamChunk, error)
}
```

### 7.3 Tool 模块

第一阶段只需支持最小 Tool 体系：

- 标准注册接口；
- 输入输出 schema；
- 超时控制；
- 执行日志；
- Tool 结果可回填给大模型。

建议：

- Tool 是 runtime 可插模块，不与人格风格绑定；
- 风格层不参与 tool 执行，只处理最终输出；
- 避免 Tool 与 Prompt 约束、人格设定互相污染。

### 7.4 Structured Response Object

这是整个系统里非常关键的一层。

不要让大模型直接吐一段散装文本，然后交给小模型随意改写。应先把回答收敛为结构化中间态。

建议结构：

```go
type StructuredResponse struct {
    FinalAnswer      string
    KeyPoints        []string
    MustKeep         []string
    RiskLevel        string
    ToolSummary      []string
    RewriteAllowed   bool
    RewriteMode      string
    RefusalBoundary  string
}
```

作用：

- 为风格层提供清晰输入；
- 保证不可改写信息可显式标注；
- 降低小模型改坏原意的风险；
- 为后续评测提供可对照对象。

### 7.5 Style Processor（小模型风格层接口）

第一阶段必须存在这个接口，即便内部先用 Prompt 改写或同模型二次调用模拟。

建议接口：

```go
type StyleProcessor interface {
    Process(ctx context.Context, resp StructuredResponse, profile StyleProfile) (string, error)
}
```

其中 `StyleProfile` 可包含：

```go
type StyleProfile struct {
    Name                string
    BluntnessLevel      int
    HumorLevel          int
    AllowSarcasm        bool
    NoEmptyEmpathy      bool
    PreserveFormat      bool
    DisableForRiskyCase bool
}
```

### 7.6 可观测性模块

必须记录：

- turn_id
- provider_name
- first_token_latency_ms
- tool_latency_ms
- style_latency_ms
- total_turn_latency_ms
- fallback_reason
- timeout_stage
- rewrite_enabled
- rewrite_mode

若没有这些数据，系统最终仍会退回“感觉哪里不舒服但不知道为什么”。

---

## 8. 双模型设计：大模型负责什么，小模型负责什么

### 8.1 大模型职责

- 用户意图理解
- 推理与计划
- Tool 调度判断
- 知识整合
- 原始回答生成
- 结构化结果归纳

### 8.2 小模型职责

- 仅在最终输出前做风格改写；
- 不重新进行任务规划；
- 不独立调用工具；
- 不擅自修改结论、数字、步骤和边界。

### 8.3 为什么要这么拆

因为当前真正想解决的问题不是“没有人格”，而是：

- 主模型默认社交策略过于礼貌；
- 容易先安抚再判断；
- 很难稳定维持“直接、尖锐、少废话”的回答风格；
- Prompt 只能刷表面语气，不能稳定改变回答哲学。

因此双模型不是为了花哨，而是为了给风格控制留一个**工程化插口**。

---

## 9. 风格层的真实目标

### 9.1 不把“人格”说得太满

本项目当前阶段的小模型层，不应该被宣传为“完整人格系统”。

更准确的定义是：

> **受控风格后处理层（controlled style post-processing layer）**

### 9.2 当前重点风格目标

你目前真正要的不是“更搞笑”，而是：

- 减少虚空安慰；
- 增加问题判断感；
- 表达更像真人说话，而不是客服公文；
- 允许适度直接、尖锐、带一点损味；
- 但不能沦为单纯骂人或乱改信息。

### 9.3 风格层必须遵守的边界

不可改写项：

- 结论
- 数字
- 时间
- 地点
- 步骤顺序
- 风险提示
- 拒绝边界
- 引用内容

允许改写项：

- 句式
- 语气
- 节奏
- 轻度吐槽/口语化
- 冗余安慰删除

---

## 10. 风格路由与适用场景

小模型风格层不能全局一把梭。

建议按场景路由：

### 10.1 场景分级

1. **普通问答**：默认 `blunt`，直接、少安慰；
2. **复盘/吐槽/策略讨论**：可用 `sharp` 或 `mean-lite`；
3. **教程/代码/操作说明**：只做轻口语化，禁止过度整活；
4. **高风险/脆弱情绪场景**：禁用冒犯式表达，但仍禁止空洞抚慰。

### 10.2 为什么必须路由

因为如果不做场景控制，系统最后会变成：

- 每个问题都一股欠揍味；
- 教程场景也开始阴阳怪气；
- 用户真正需要严肃信息时被风格盖住。

这不是风格鲜明，是系统性失真。

---

## 11. 目录结构建议（v0.1）

```text
agent/
├── cmd/
│   └── server/
│       └── main.go
├── internal/
│   ├── runtime/
│   │   ├── engine.go
│   │   ├── session.go
│   │   ├── stream.go
│   │   └── metrics.go
│   ├── llm/
│   │   ├── client.go
│   │   ├── openai.go
│   │   ├── anthropic.go
│   │   └── ollama.go
│   ├── response/
│   │   ├── structured.go
│   │   └── formatter.go
│   ├── style/
│   │   ├── processor.go
│   │   ├── prompt_rewriter.go
│   │   └── local_model.go
│   ├── tool/
│   │   ├── registry.go
│   │   ├── executor.go
│   │   └── types.go
│   ├── prompt/
│   │   ├── builder.go
│   │   └── templates.go
│   ├── memory/
│   │   ├── short.go
│   │   └── summary.go
│   └── obs/
│       ├── logger.go
│       └── trace.go
├── api/
│   ├── http.go
│   └── websocket.go
├── configs/
│   └── config.yaml
└── go.mod
```

---

## 12. 第一阶段实施建议

### 12.1 第一步：搭最小 runtime

先完成：

- 单会话输入输出
- 大模型调用
- 流式输出
- 一个 Tool
- 基础日志

### 12.2 第二步：加入 Structured Response Object

这一步完成后，主模型输出不再是单纯散文，而是：

- 核心结论
- 关键点
- 必保留项
- 风格是否允许

### 12.3 第三步：插入 Style Processor 接口

即使先用最简单的 Prompt 改写，也要按正式接口接进去。

### 12.4 第四步：加入风格路由

不要默认所有回答都走风格层，先根据场景决定是否改写、改写强度、是否允许 sarcasm。

### 12.5 第五步：做最小评测闭环

至少对比：

- 原始回答
- 风格层回答
- 是否改坏信息
- 用户主观观感
- 平均延迟变化

---

## 13. 评测框架建议

### 13.1 Runtime 指标

- 首 token 延迟
- 总响应时间
- Tool 成功率
- Tool 平均耗时
- 流式中断率
- WS/SSE 重连耗时

### 13.2 风格层指标

- 核心信息保真率
- 虚空共情率
- 明确结论率
- 风格保持率
- 场景错配率
- 用户主观评分（真诚 / 刺耳 / 有帮助 / 装凶）

### 13.3 最关键的两个问题

你第一阶段其实只需要验证两件事：

1. **Go runtime 能否明显比现有 OpenClaw 体感更稳、更顺？**
2. **小模型风格层能否作为独立 stage 插入，而不把内容改坏？**

若这两件事都不能证明，后续做复杂记忆、复杂 UI、复杂技能都意义不大。

---

## 14. 风险与常见误区

### 14.1 误区一：一上来想复刻 OpenClaw 全家桶

这会导致项目迅速膨胀，最后既没有轻 runtime，也没有稳定 style hook。

### 14.2 误区二：把“更像真人”误写成“更有用”

用户真正讨厌的往往不是模型不够像人，而是：

- 不敢判断
- 不敢说错
- 把礼貌当帮助
- 先安慰再解决

### 14.3 误区三：把小模型风格层神化

小模型确实更容易做出风格，但也更容易：

- 改坏原意
- 漏条件
- 加戏
- 在不合适场景嘴臭

所以它必须是受控组件，而不是自由发挥的第二脑。

### 14.4 误区四：没有可观测性就谈体感优化

没有指标，最后只会停留在“感觉还是卡”“好像顺了一点”的模糊感受。

---

## 15. 参考项目与借鉴建议

以下项目适合作为“拆骨架参考”，而不是整套照抄。重点是借鉴 **runtime、provider abstraction、tool boundary、可观测性与轻量工程结构**。

### 15.1 Routex

- GitHub: https://github.com/Ad3bay0c/routex
- 定位：轻量 Go AI agent runtime；
- 可借鉴点：
  - 使用 goroutines 和 channels 组织运行时；
  - 强调 scheduling、parallelism、retries、observability；
  - 支持 OpenAI / Anthropic / Ollama；
  - 有 CLI / YAML 驱动思路，适合参考 runtime 外壳设计。
- 适合借鉴：**runtime 骨架**
- 不建议直接照抄的点：若项目后期继续膨胀成复杂 crew/graph，容易偏离你当前“轻单体 runtime”目标。

### 15.2 llm-sdk / LLM Agent

- GitHub: https://github.com/hoangvvo/llm-sdk
- 定位：统一 LLM provider API + 最小透明 agent library；
- 可借鉴点：
  - 统一 `LanguageModel` 风格接口；
  - 模型与 tool orchestration 的薄抽象；
  - “没有 hidden prompt / secret sauce”的透明思路非常适合你当前需求。
- 适合借鉴：**provider abstraction + minimal agent API**
- 不建议直接照抄的点：它更像通用 SDK，不会直接给你完整 runtime 产品壳。

### 15.3 Phero

- GitHub: https://github.com/henomis/phero
- 定位：现代 Go 多 Agent 框架；
- 可借鉴点：
  - agent / skill / memory / guardrails / tracing 模块化切分；
  - 对生产环境能力的重视更强；
  - 适合作为“中大型工程切分方式”的样板。
- 适合借鉴：**模块边界与工程切分**
- 不建议直接照抄的点：它已经开始走完整框架路线，容易把你重新带回“大而全”陷阱。

### 15.4 CloudWeGo Eino

- GitHub: https://github.com/cloudwego/eino
- Examples: https://github.com/cloudwego/eino-examples
- 定位：Go 生态中的 LLM 应用开发框架；
- 可借鉴点：
  - `ChatModel` / `Tool` / `Retriever` / Template 等抽象比较正式；
  - 组件化思路清晰；
  - 适合参考企业级风格的 framework 设计。
- 适合借鉴：**组件抽象与 workflow 设计**
- 不建议直接照抄的点：对当前阶段而言偏重，容易把项目做成平台，而不是 daily-driver runtime。

### 15.5 Hector / tRPC-Agent-Go / go-agent-framework

- Hector: https://github.com/verikod/hector
- tRPC-Agent-Go: https://github.com/trpc-group/trpc-agent-go
- go-agent-framework: https://github.com/stephanoumenos/go-agent-framework
- 这些项目的价值主要在于：
  - 看它们如何做单二进制、配置驱动、工作流依赖解析、错误处理；
  - 看“Go 生态里 agent framework 会长成什么样”。
- 适合借鉴：**工程结构、workflow 抽象、配置组织方式**
- 不建议直接照抄的点：容易把注意力从“runtime 顺手”偏到“框架功能炫技”。

### 15.6 Hermes Agent

- GitHub: https://github.com/NousResearch/hermes-agent
- 定位：带 learning / memory / skill 野心的 agent 系统；
- 适合借鉴：**产品形态和能力边界想象**
- 不建议直接照抄：它更适合作为产品层对照，而不是你第一阶段的 runtime 模板。

### 15.7 OpenClaw

- GitHub: https://github.com/openclaw/openclaw
- 适合借鉴：
  - Skill 组织习惯；
  - channel / assistant 的产品思路；
  - 持续在线 agent 的使用方式。
- 不建议直接延续：
  - 当前正是因为你对其体感、连接、链路不满意才要重构；
  - 不应在新项目里继承它的整体复杂度和心智负担。

### 15.8 参考项目使用策略总结

建议优先级：

1. **Routex + llm-sdk**：最适合拿来搭第一版骨架；
2. **Phero**：拿来学模块切分和 tracing；
3. **Eino / Hector / tRPC-Agent-Go / go-agent-framework**：当作工程素材库；
4. **Hermes / OpenClaw**：作为产品形态对照，而不是实现模板。

---

## 16. 建议的 MVP 定义

### 16.1 MVP 一句话定义

> **一个 Go 写的、流式稳定、支持至少一个 Tool 和一个 Style Hook 的最小 Agent runtime。**

### 16.2 MVP 必须具备

- 稳定网络连接
- 基础多 provider 支持
- Structured Response 中间态
- Style Processor 接口
- 基础日志与耗时观测

### 16.3 MVP 暂不做

- 复杂多 Agent 图
- 全量记忆
- 完整 Dashboard
- 完整 OpenClaw skill 兼容层
- 成熟小模型训练系统

---

## 17. ClawHub Skill 集成

### 17.1 背景

ClawHub 是 OpenClaw 生态的 skill 注册中心，skill 以目录形式发布，每个目录包含一个 `SKILL.md` 文件（YAML frontmatter + markdown body）及可选支撑文件。本项目通过 skill loader 实现与 ClawHub skill 格式的兼容，无需运行 OpenClaw 本体。

### 17.2 Skill 文件格式

```
skills/
└── todoist-cli/
    ├── SKILL.md        # 必需
    └── （可选）其他文本文件
```

`SKILL.md` 结构：

```markdown
---
name: todoist-cli
description: Manage Todoist tasks from the command line.
version: 1.2.0
metadata:
  openclaw:
    requires:
      env: [TODOIST_API_KEY]
      bins: [todoist-cli]
    primaryEnv: TODOIST_API_KEY
---
# Todoist CLI Skill

When the user asks to manage tasks, use the `todoist-cli` binary...
（操作步骤、输入说明、错误处理等 runbook 内容）
```

### 17.3 两类 Skill 的处理方式

| 类型 | 判断条件 | 处理方式 |
|------|----------|----------|
| 纯提示型 | `bins` 字段为空 | body 注入 system prompt |
| CLI 工具型 | `bins` 字段有值 | 注册为 `Tool`（handler 调对应 CLI）+ body 注入 system prompt |

两类 skill 都会将 body 注入 system prompt，确保 agent 知道该如何使用该能力。CLI 类 skill 额外注册成可调用工具，LLM 可通过 tool use 显式触发。

### 17.4 实现位置

```
agent/
├── skills/                          # ClawHub skill 目录，直接解压放这里
│   └── <skill-name>/
│       └── SKILL.md
└── internal/tool/
    └── skill_loader.go              # 扫描、解析、注册
```

核心函数：
- `LoadSkillsDir(ctx, dir)` → `*SkillLoadResult`：扫描目录，返回工具列表和 prompt 列表
- `BuildSkillSystemPrompt(base, prompts)` → `string`：将 skill body 追加到 base system prompt

### 17.5 启动时行为

- 扫描 `config.yaml` 里 `skills.dir` 指定的目录（空 = 禁用）
- 缺失 CLI binary 或未设置 env var：打 warn 日志，**不阻止启动**
- 注册成功的工具通过 `registry.Definitions()` 自动暴露给 LLM

### 17.6 限制与边界

- `install` 字段声明的依赖（brew/node/go 包）**不会自动安装**，需提前手动准备环境
- 不兼容 OpenClaw 的 channel/session 机制，仅复用 skill 的 runbook 内容和 CLI 调用
- skill body 会增加每轮 system prompt 长度，skill 数量多时注意 context 窗口压力

---

## 18. 最后结论

这条路线是成立的，而且比“继续磨嘴臭 Prompt”更靠谱。

本项目的真正价值不在于“做一个更像人的 AI”，而在于：

1. **重构一个自己真正愿意用的 Agent runtime**；
2. **把风格层做成明确的系统插口，而不是 Prompt 小花招**；
3. **让回答先有判断，再谈表达**；
4. **避免继续被过度礼貌、过度安抚、状态不透明的系统折磨。**

最该坚持的边界是：

> 先做一个顺手、稳定、可插 style processor 的 Go runtime，别急着造第二个 OpenClaw。

---

## 19. 参考来源（用于项目调研）

- Routex GitHub: https://github.com/Ad3bay0c/routex
- Routex doc / pkg.go.dev: https://pkg.go.dev/github.com/Ad3bay0c/routex/cmd/routex
- llm-sdk GitHub: https://github.com/hoangvvo/llm-sdk
- llm-sdk Go package: https://pkg.go.dev/github.com/hoangvvo/llm-sdk/sdk-go
- Phero GitHub: https://github.com/henomis/phero
- CloudWeGo Eino GitHub: https://github.com/cloudwego/eino
- Eino Examples: https://github.com/cloudwego/eino-examples
- Hector GitHub: https://github.com/verikod/hector
- tRPC-Agent-Go GitHub: https://github.com/trpc-group/trpc-agent-go
- go-agent-framework GitHub: https://github.com/stephanoumenos/go-agent-framework
- Hermes Agent GitHub: https://github.com/NousResearch/hermes-agent
- OpenClaw GitHub: https://github.com/openclaw/openclaw


一些补充：

十、风格层接口契约（Style Processor Contract）

在本架构中，本地小模型不作为主推理模型参与任务理解、工具调用与知识生成，而是作为 受控风格改写服务 接入 Runtime，在最终输出阶段执行表达层处理。其核心职责是：在不改变结论、事实、步骤与安全边界的前提下，对输出进行风格化改写。

10.1 角色定位

Style Processor 的定位如下：

不参与用户意图理解
不参与技能调度
不参与工具调用
不负责补充新知识
不负责纠正上游推理错误
仅负责对上游结构化结果进行风格层改写

这意味着，小模型是 后处理器（post-processor），而不是第二个主模型。

10.2 接口抽象

建议在 Go Runtime 中定义统一接口：

type StyleProcessor interface {
    Rewrite(ctx context.Context, req StyleRewriteRequest) (StyleRewriteResponse, error)
}
10.3 请求结构
type StyleRewriteRequest struct {
    TurnID             string
    FinalAnswer        string
    KeyPoints          []string
    MustKeep           []string
    RiskLevel          string
    StyleProfile       string
    RewriteMode        string
    Constraints        []string
    Metadata           map[string]string
}

字段说明：

TurnID：当前轮次唯一标识，用于日志与链路追踪
FinalAnswer：上游大模型产出的核心回答
KeyPoints：关键要点列表，用于辅助改写时保持信息骨架
MustKeep：不可丢失的信息项
RiskLevel：风险等级，如 low / medium / high
StyleProfile：风格档位，如 plain / blunt / sharp / mean-lite
RewriteMode：改写模式，如 light_rewrite / strong_rewrite / bypass
Constraints：改写约束条件
Metadata：扩展字段，例如场景类型、工具使用情况等
10.4 返回结构
type StyleRewriteResponse struct {
    OutputText         string
    Applied            bool
    AppliedProfile     string
    ValidationPassed   bool
    FallbackReason     string
    Diagnostics        map[string]string
}

字段说明：

OutputText：最终风格化结果
Applied：是否实际应用风格层
AppliedProfile：实际应用的风格档位
ValidationPassed：是否通过改写校验
FallbackReason：未应用时的回退原因
Diagnostics：调试与监控字段
10.5 部署方式

小模型建议作为独立本地服务运行，通过统一 API 接入，支持以下后端：

Ollama
llama.cpp server
OpenAI-compatible local service
自封装 HTTP / gRPC 推理服务

优点如下：

与主 Runtime 解耦
可替换底座模型
可独立监控与重启
风格层失败时易于回退
后续接入 LoRA 或不同风格模型时无需修改主链路
十一、Structured Response 中间态设计

为避免小模型直接处理散装自由文本，主架构引入 Structured Response 作为大模型与风格层之间的中间态。该结构是双模型解耦的核心约束之一。

11.1 设计目标

Structured Response 的目标是：

将“大模型负责内容、小模型负责表达”从理念变为结构约束
防止小模型在自由改写中篡改结论或遗漏关键信息
为后续校验、回退、日志、评测提供标准化输入
11.2 推荐结构
type StructuredResponse struct {
    FinalAnswer   string
    KeyPoints     []string
    MustKeep      []string
    RiskLevel     string
    StyleAllowed  bool
    RewriteMode   string
    SceneType     string
    ToolUsed      bool
}
11.3 字段说明
FinalAnswer：主回答内容
KeyPoints：核心信息骨架
MustKeep：不可变更项
RiskLevel：风险等级
StyleAllowed：本轮是否允许风格改写
RewriteMode：改写强度
SceneType：场景类型，如 chat / tutorial / coding / risky / emotional
ToolUsed：是否包含工具调用结果
11.4 运行逻辑

建议主流程如下：

用户输入
→ Runtime 路由
→ 大模型推理 / 工具调用
→ 生成 Structured Response
→ 风格路由判断
→ 调用 Style Processor
→ 输出最终文本

这样做的关键价值在于：
风格层不直接面向原始上下文，而只面向经过整理的、可控的中间态对象。

十二、不可改写项（Non-Rewritable Constraints）

为防止风格层在改写过程中破坏信息完整性，系统需要定义明确的不可改写项。
“只改语气不改信息”不能只作为口头原则，必须落为明确约束。

12.1 默认不可改写项

以下内容默认不可改写：

数字信息
包括数量、金额、时间长度、版本号、比例、评分等
时间与地点信息
包括日期、截止时间、时区、位置、路径等
操作步骤
包括教程步骤、命令顺序、执行流程、配置说明等
风险提示
包括警告、限制条件、失败后果、兼容性提醒等
拒绝边界
包括不能做、不能提供、不能判断、无法确认等表述
引用内容
包括用户原话、外部引用、日志片段、错误信息
代码块与配置片段
默认不进入风格化改写流程
12.2 允许改写项

以下内容允许在约束下改写：

句式与语序
口语化程度
幽默感、犀利感、吐槽感
节奏与断句
非关键修饰语
12.3 风格层约束模板

建议每次调用风格层时显式携带如下约束：

不得新增事实
不得删除关键条件
不得改变结论强度
不得弱化风险提示
不得修改数字/时间/地点
不得改写代码块
十三、风格路由规则（Style Routing）

风格化输出不是全局默认开启，而应由 Runtime 根据场景、风险等级、输出类型进行路由判断。

13.1 路由原则

系统默认遵循以下原则：

先保证任务完成，再考虑表达风格
高风险场景优先保守
风格增强只作用于适合的对话类型
避免全局统一“嘴臭化”
13.2 推荐场景映射
1）闲聊 / 复盘 / 吐槽 / 讨论类

适用风格：

blunt
sharp
mean-lite

特点：

可以减少安慰
可以增强判断感
可以加入轻度吐槽与冒犯式提醒
允许更强烈的风格表达
2）教程 / 操作说明 / 代码解释

适用风格：

plain
blunt

特点：

重点是清晰和保真
禁止过度整活
可减少客套，但不宜加入过强攻击性
3）高风险 / 敏感 / 脆弱情绪场景

适用风格：

plain
blunt

特点：

仍减少虚空安慰
但禁止明显冒犯和嘲讽
以直接、克制、清晰为主
13.3 伪代码示例
func ResolveStyleProfile(sceneType string, riskLevel string) string {
    if riskLevel == "high" {
        return "plain"
    }

    switch sceneType {
    case "chat", "reflection", "discussion":
        return "sharp"
    case "tutorial", "coding":
        return "blunt"
    case "emotional":
        return "plain"
    default:
        return "blunt"
    }
}
十四、Fallback 与异常处理机制

小模型风格层作为附加能力，不能成为主回答链路的单点故障源。
因此系统必须具备明确的回退机制。

14.1 回退原则
风格层失败不应影响主回答可用性
小模型异常时，应优先返回上游原始回答
所有风格层错误都必须可观测、可记录
14.2 触发回退的典型情况
小模型服务不可用
调用超时
返回格式不合法
改写结果校验失败
结果丢失关键字段
风格路由判定为不适合改写
14.3 回退策略
策略 A：直接回退原始回答

适用于绝大多数情况

Final Output = StructuredResponse.FinalAnswer
策略 B：降级为轻改写

当强风格模型失败，但轻风格模板可用时，可退到 plain 或 blunt

策略 C：完全跳过风格层

在高风险场景中，可直接 bypass

14.4 日志记录建议

每次风格层调用建议记录：

turn_id
style_profile_expected
style_profile_applied
style_latency_ms
fallback_triggered
fallback_reason
validation_passed
十五、评测指标与可观测性

本项目的核心目标之一是改善 OpenClaw 类框架中“连接卡顿、响应拖沓、风格不稳、对话发虚”的使用体验。因此必须建立显式指标，而不能仅依赖主观感受。

15.1 Runtime 指标
首 token 延迟（First Token Latency）
衡量流式响应的第一时间反馈能力
总轮次耗时（Total Turn Latency）
衡量单轮完整处理耗时
风格层额外耗时（Style Layer Overhead）
衡量接入小模型后的额外延迟成本
小模型失败率（Style Processor Failure Rate）
衡量风格层稳定性
Fallback Rate
衡量风格层不可用时的回退频率
15.2 内容质量指标
核心信息保真率
改写前后关键结论、步骤、数字是否保持一致
风格命中率
输出是否符合预期风格档位
虚空共情率
输出中是否出现无信息量安慰性套话
判断明确率
是否明确指出问题、风险或结论，而非模糊表述
场景错配率
是否在不合适场景中使用过强风格
15.3 用户体验指标

可通过人工标注或小规模偏好测试评估：

是否更直接
是否更少废话
是否更少“AI客服假笑”
是否更像有判断的人
是否虽尖锐但仍有帮助
十六、参考项目补充说明（用于重构与复现）

为避免从零设计造成方向发散，本项目建议参考现有 GitHub 项目的局部实现方式，但不直接照搬其全部能力边界。

16.1 优先参考的项目类型
1）轻量 Go Agent Runtime

适合参考主循环、并发调度、流式输出、错误恢复

2）最小透明抽象类项目

适合参考 provider 封装、tool 调用、session 管理、API 设计

3）完整 Go Agent Framework

适合参考模块分层、memory、skills、trace、guardrails 等设计方式，但不建议第一版整套继承

16.2 借鉴原则

建议仅借鉴以下层面：

runtime 主循环
provider 抽象
style hook 预留方式
skill 注册与调用边界
trace / logging / metrics
memory 分层接口

不建议直接照搬以下内容：

过重的多 agent 编排
过早引入复杂 graph
平台化配置系统
大而全 UI 壳
无法解释的黑盒 prompt orchestration
16.3 当前重构目标

当前阶段的目标不是复刻 OpenClaw 全功能，而是先完成以下最小闭环：

Go Runtime 稳定接收输入
大模型流式输出稳定
支持基础 tool 调用
预留本地小模型风格接口
风格层失败可回退
链路可观测

该目标优先于复杂技能系统、长期记忆优化与人格反馈系统。
