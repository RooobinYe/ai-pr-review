# AI PR Review 助手

<p align="center">
  <strong>题目三：AI PR Review 助手</strong><br/>
  一个基于 AI 的代码评审工具，帮助开发者提升 Pull Request 的 Review 效率与质量。
</p>

---

## 项目简介

在日常开发中，Pull Request Review 往往成为团队协作的瓶颈——Reviewer 需要花费大量时间理解代码变更、识别潜在风险、给出建设性建议。AI PR Review 助手旨在通过 AI 辅助分析，自动化 PR 评审中的重复性工作，让 Reviewer 能聚焦于最关键的设计决策和逻辑正确性。

### 核心功能

- **PR 变更总结**：自动获取 PR 的代码变更，生成结构化的变更摘要，包括变更范围、影响模块、核心逻辑变化。
- **风险代码识别**：基于 AST 分析和规则引擎，识别潜在的安全漏洞、性能问题、并发风险、空指针等常见代码缺陷。
- **Review 建议生成**：针对识别出的问题，生成具体、可操作的 Review 建议，包括问题描述、严重程度、修复方案。

### 设计思路

#### 模型选择

系统支持多种 LLM 后端（Anthropic Claude、OpenAI GPT 系列），可根据任务复杂度灵活切换：
- **变更总结**：使用成本较低的模型（如 Claude Haiku），快速生成摘要
- **风险识别**：使用平衡型模型（如 Claude Sonnet），兼顾准确性和速度
- **深度分析**：对于复杂 PR，支持切换到最强模型（如 Claude Opus）进行深度审查

#### 上下文获取方式

- **结构化 Diff 解析**：将 Git diff 解析为结构化数据，按文件、函数粒度组织
- **AST 静态分析**：对变更代码进行语法树分析，识别代码模式
- **关联文件加载**：自动识别变更文件的依赖关系，加载相关上下文

#### 未来扩展方向

- [ ] 仓库级代码知识图谱，提升跨文件分析准确性
- [ ] Review 规则自定义，支持团队编码规范配置
- [ ] CI/CD 集成，PR 创建时自动触发 Review
- [ ] Review 历史学习，减少重复性误报

---

## 快速开始

### 前置条件

- Go 1.24+
- GitHub Personal Access Token（用于访问 PR 信息）
- Anthropic API Key 或 OpenAI API Key

### 安装

```sh
git clone https://github.com/yezhenrong/ai-pr-review
cd ai-pr-review
go build -o ai-pr-review ./cmd/ai-pr-review
```

### 使用

```sh
# 评审指定 PR
./ai-pr-review --pr https://github.com/owner/repo/pull/123

# 指定模型
./ai-pr-review --pr https://github.com/owner/repo/pull/123 --model claude-sonnet-4-20250514

# 输出 JSON 格式结果
./ai-pr-review --pr https://github.com/owner/repo/pull/123 --format json
```

### 环境变量

| 变量 | 说明 |
|------|------|
| `GITHUB_TOKEN` | GitHub Personal Access Token |
| `ANTHROPIC_API_KEY` | Anthropic API Key |
| `OPENAI_API_KEY` | OpenAI API Key |

---

## 项目结构

```
ai-pr-review/
├── cmd/ai-pr-review/        # CLI 入口
└── internal/
    ├── api/                 # LLM API 客户端（支持 Anthropic / OpenAI）
    ├── auth/                # 认证与凭证管理
    ├── config/              # 配置加载与管理
    ├── context/             # 上下文组装（Git diff 解析、依赖分析）
    ├── review/              # Review 引擎（变更总结、风险识别、建议生成）
    ├── tools/               # 工具实现（文件读取、搜索等）
    └── tui/                 # 终端 UI
```

---

## 许可证

MIT
