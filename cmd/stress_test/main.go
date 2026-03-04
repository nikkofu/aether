package main

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"time"

	"github.com/nikkofu/aether/internal/core/economy"
)

func main() {
	fmt.Println("开始执行 AI 经济系统与自治调度压力测试...")

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		panic(err)
	}
	defer db.Close()

	ledger, err := economy.NewSQLiteLedger(db)
	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	orgID := "test-org"

	// 1. 初始化 100 个 Agent
	agentIDs := make([]string, 100)
	for i := 0; i < 100; i++ {
		agentIDs[i] = fmt.Sprintf("agent-%03d", i)
		ledger.GetAccount(ctx, orgID, agentIDs[i])
	}

	// 2. 模拟 10000 次执行
	rand.Seed(time.Now().UnixNano())
	for i := 0; i < 10000; i++ {
		selectedAgent := selectAgentByReputation(ctx, ledger, orgID, agentIDs)

		acc, _ := ledger.GetAccount(ctx, orgID, selectedAgent)
		successRate := 0.5 + (acc.Reputation / 200.0)
		if successRate > 0.9 {
			successRate = 0.9
		}

		cost := rand.Float64()*0.1 + 0.05

		ledger.UpdateBalance(ctx, orgID, selectedAgent, -cost, 0)
		ledger.AddTransaction(ctx, economy.Transaction{
			ID: fmt.Sprintf("tx-cost-%d", i), OrgID: orgID, From: selectedAgent, To: "system", Amount: cost, Type: "cost", CreatedAt: time.Now(),
		})

		isSuccess := rand.Float64() < successRate
		if isSuccess {
			reward := cost * 0.2
			ledger.UpdateBalance(ctx, orgID, selectedAgent, reward, 1.0)
			ledger.AddTransaction(ctx, economy.Transaction{
				ID: fmt.Sprintf("tx-rew-%d", i), OrgID: orgID, From: "system", To: selectedAgent, Amount: reward, Type: "reward", CreatedAt: time.Now(),
			})
		} else {
			ledger.UpdateBalance(ctx, orgID, selectedAgent, 0, -1.0)
		}

		if i%1000 == 0 && i != 0 {
			ledger.ApplyReputationDecay(ctx, orgID, 0.02)
			ledger.BurnExcessTokens(ctx, orgID, 5000.0)
		}
	}

	// 3. 统计输出
	fmt.Println("\n--- 压测结果 (执行 10000 次) ---")
	
	topAgents, _ := ledger.TopAgentsByReputation(ctx, orgID, 10)
	fmt.Println("\n【Top 10 高信誉 Agent (优胜者)】")
	for i, a := range topAgents {
		fmt.Printf("%2d. %s | 信誉: %6.2f | 余额: $%6.2f\n", i+1, a.AgentID, a.Reputation, a.Balance)
	}

	var totalBalance, totalRep float64
	for _, id := range agentIDs {
		a, _ := ledger.GetAccount(ctx, orgID, id)
		totalBalance += a.Balance
		totalRep += a.Reputation
	}
	
	fmt.Printf("\n【宏观经济指标】\n")
	fmt.Printf("系统总发币量: $%6.2f\n", totalBalance)
	fmt.Printf("系统总信誉值: %6.2f\n", totalRep)
	
	if len(topAgents) > 0 && topAgents[0].Balance > totalBalance*0.5 {
		fmt.Println("状态: 出现严重寡头垄断！")
	} else {
		fmt.Println("状态: 经济生态健康，阶层流动正常。")
	}
}

func selectAgentByReputation(ctx context.Context, ledger economy.Ledger, orgID string, ids []string) string {
	var totalWeight float64
	weights := make([]float64, len(ids))
	for i, id := range ids {
		acc, _ := ledger.GetAccount(ctx, orgID, id)
		w := acc.Reputation
		if w <= 0 { w = 0.1 }
		weights[i] = w
		totalWeight += w
	}

	r := rand.Float64() * totalWeight
	for i, w := range weights {
		r -= w
		if r <= 0 {
			return ids[i]
		}
	}
	return ids[0]
}
