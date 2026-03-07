module github.com/OctoSucker/octosucker

go 1.24.4

toolchain go1.24.10

require (
	github.com/OctoSucker/octosucker-skill v0.0.0
	github.com/OctoSucker/skill-fs v0.0.0
	github.com/OctoSucker/skill-telegram v0.0.0-00010101000000-000000000000
	github.com/OctoSucker/skill-web v0.0.0
	github.com/google/uuid v1.6.0
	github.com/openai/openai-go v1.12.0
)

require (
	github.com/go-rod/rod v0.116.2 // indirect
	github.com/go-telegram-bot-api/telegram-bot-api/v5 v5.5.1 // indirect
	github.com/tidwall/gjson v1.14.4 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/ysmood/fetchup v0.2.3 // indirect
	github.com/ysmood/goob v0.4.0 // indirect
	github.com/ysmood/got v0.40.0 // indirect
	github.com/ysmood/gson v0.7.3 // indirect
	github.com/ysmood/leakless v0.9.0 // indirect
	golang.org/x/net v0.34.0 // indirect
)

replace github.com/google-agentic-commerce/a2a-x402 => /Users/zecrey/Desktop/yiming/a2a-x402/golang

// 本地开发
replace github.com/OctoSucker/octosucker-skill => ../octosucker-skill

replace github.com/OctoSucker/skill-agent-chat => ../skill-agent-chat

replace github.com/OctoSucker/octosucker-utils => ../octosucker-utils

replace github.com/OctoSucker/skill-fs => ../skill-fs

replace github.com/OctoSucker/skill-telegram => ../skill-telegram

replace github.com/OctoSucker/skill-web => ../skill-web
