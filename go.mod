module github.com/OctoSucker/octosucker

go 1.24.4

toolchain go1.24.10

require (
	github.com/OctoSucker/octosucker-skill v0.0.0
	github.com/OctoSucker/skill-telegram v0.0.0-00010101000000-000000000000
	github.com/google/uuid v1.6.0
	github.com/openai/openai-go v1.12.0
)

require (
	github.com/go-telegram-bot-api/telegram-bot-api/v5 v5.5.1 // indirect
	github.com/tidwall/gjson v1.14.4 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
)

replace github.com/google-agentic-commerce/a2a-x402 => /Users/zecrey/Desktop/yiming/a2a-x402/golang

// 本地开发
replace github.com/OctoSucker/octosucker-skill => ../octosucker-skill

replace github.com/OctoSucker/skill-agent-chat => ../skill-agent-chat

replace github.com/OctoSucker/octosucker-utils => ../octosucker-utils

replace github.com/OctoSucker/skill-telegram => ../skill-telegram
