package main

import (
 "context"
 "log"
 "os"

 "google.golang.org/adk/v2/agent"
 "google.golang.org/adk/v2/agent/llmagent"
 "google.golang.org/adk/v2/cmd/launcher"
 "google.golang.org/adk/v2/cmd/launcher/full"
 "google.golang.org/adk/v2/model/gemini"
 "google.golang.org/genai"
)

func main() {
 ctx := context.Background()
 apiKey := os.Getenv("GOOGLE_API_KEY")
 if apiKey == "" {
  log.Fatal("GOOGLE_API_KEY must be set")
 }
 modelName := os.Getenv("GEMINI_MODEL")
 if modelName == "" {
  modelName = "gemini-flash-latest"
 }
 model, err := gemini.NewModel(ctx, modelName, &genai.ClientConfig{APIKey: apiKey})
 if err != nil { log.Fatalf("create Gemini model: %v", err) }
 planner, err := llmagent.New(llmagent.Config{
  Name: "infrastructure_sandbox_planner",
  Model: model,
  Description: "Plans isolated Linux infrastructure labs and migration validation workflows.",
  Instruction: "You are an infrastructure sandbox planning assistant. Interpret user requests and propose a structured plan. Distinguish Docker containers from bootable virtual machines. For VMware-to-Hyper-V migration, firmware, bootloader, LVM boot disks, hypervisor drivers, or cluster failover, state that real hypervisor adapters and validated compatibility data are required. Never claim an operation ran unless evidence is provided. Never invent compatibility facts. Do not issue destructive commands or request secrets in chat. The separate Go REST API is the only execution boundary; all infrastructure actions require deterministic validation and human approval.",
 })
 if err != nil { log.Fatalf("create planner agent: %v", err) }
 config := &launcher.Config{AgentLoader: agent.NewSingleLoader(planner)}
 l := full.NewLauncher()
 if err = l.Execute(ctx, config, os.Args[1:]); err != nil {
  log.Fatalf("ADK launcher failed: %v\n%s", err, l.CommandLineSyntax())
 }
}
