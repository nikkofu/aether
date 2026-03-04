package observability

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	ActiveAgents = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aether_active_agents",
		Help: "当前系统中的活跃代理总数",
	})

	TotalTransactions = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aether_total_transactions_total",
		Help: "经济系统处理的消息交易总数",
	})

	TotalProposals = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aether_total_proposals_total",
		Help: "治理系统创建的提案总数",
	})

	AverageReputation = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aether_average_reputation",
		Help: "全系统代理的平均信誉值",
	})

	TokenSupply = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aether_token_supply",
		Help: "系统当前的代币流通总量",
	})
)

// StartMetricsServer 启动 Prometheus 指标暴露服务。
func StartMetricsServer(addr string) error {
	http.Handle("/metrics", promhttp.Handler())
	return http.ListenAndServe(addr, nil)
}
