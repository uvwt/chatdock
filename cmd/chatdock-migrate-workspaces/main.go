package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"chatdock/internal/chatdock/legacyworkspace"
)

func main() {
	var options legacyworkspace.LegacyWorkspaceMigrationOptions
	flag.StringVar(&options.SourcePath, "source", "", "旧工作空间 SQLite 独立快照路径")
	flag.StringVar(&options.TargetPath, "target", "", "新项目结构 SQLite 输出路径；必须不存在")
	flag.StringVar(&options.GlobalWorkspace, "global-workspace", "default", "迁移为全局配置和普通会话的旧工作空间")
	flag.Parse()

	report, err := legacyworkspace.MigrateLegacyWorkspaces(options)
	if err != nil {
		fmt.Fprintln(os.Stderr, "迁移失败:", err)
		os.Exit(1)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintln(os.Stderr, "输出迁移报告失败:", err)
		os.Exit(1)
	}
}
