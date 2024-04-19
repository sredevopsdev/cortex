package ruler

import (
	"context"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	io_prometheus_client "github.com/prometheus/client_model/go"
	promRules "github.com/prometheus/prometheus/rules"
	"github.com/stretchr/testify/require"

	"github.com/cortexproject/cortex/pkg/ruler/rulespb"
	"github.com/cortexproject/cortex/pkg/util"
)

func TestBackUpRuleGroups(t *testing.T) {
	r := rulespb.RuleDesc{
		Expr:   "1 > bool 1",
		Record: "test",
	}
	g1 := rulespb.RuleGroupDesc{
		Name:      "g1",
		Namespace: "ns1",
		Rules:     []*rulespb.RuleDesc{&r},
	}
	g2 := rulespb.RuleGroupDesc{
		Name:      "g2",
		Namespace: "ns1",
		Rules:     []*rulespb.RuleDesc{&r},
	}
	g3 := rulespb.RuleGroupDesc{
		Name:      "g3",
		Namespace: "ns2",
		Rules:     []*rulespb.RuleDesc{&r},
	}

	cfg := defaultRulerConfig(t)
	manager := newRulesBackupManager(cfg, log.NewNopLogger(), nil)

	type testCase struct {
		input map[string]rulespb.RuleGroupList
	}

	testCases := map[string]testCase{
		"Empty input": {
			input: make(map[string]rulespb.RuleGroupList),
		},
		"With groups from single users": {
			input: map[string]rulespb.RuleGroupList{
				"user2": {&g1, &g2},
			},
		},
		"With groups from multiple users": {
			input: map[string]rulespb.RuleGroupList{
				"user1": {&g1, &g3},
				"user2": {&g1, &g2},
			},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			manager.setRuleGroups(context.TODO(), tc.input)
			require.Equal(t, len(tc.input), len(manager.inMemoryRuleGroupsBackup))
			for user, groups := range tc.input {
				loadedGroups := manager.getRuleGroups(user)
				require.Equal(t, groups, loadedGroups)
			}
		})
	}
}

func TestBackUpRuleGroupsMetrics(t *testing.T) {
	r := rulespb.RuleDesc{
		Expr:   "1 > bool 1",
		Record: "test",
	}
	g1 := rulespb.RuleGroupDesc{
		Name:      "g1",
		Namespace: "ns1",
		Rules:     []*rulespb.RuleDesc{&r},
	}
	g2 := rulespb.RuleGroupDesc{
		Name:      "g2",
		Namespace: "ns1",
		Rules:     []*rulespb.RuleDesc{&r},
	}
	g2Updated := rulespb.RuleGroupDesc{
		Name:      "g2",
		Namespace: "ns1",
		Rules:     []*rulespb.RuleDesc{&r, &r},
	}

	cfg := defaultRulerConfig(t)
	reg := prometheus.NewPedanticRegistry()
	manager := newRulesBackupManager(cfg, log.NewNopLogger(), reg)

	getGroupName := func(g *rulespb.RuleGroupDesc, user string) string {
		dirPath := filepath.Join(cfg.RulePath, user)
		encodedFileName := url.PathEscape(g.Namespace)
		path := filepath.Join(dirPath, encodedFileName)
		return promRules.GroupKey(path, g.Name)
	}

	manager.setRuleGroups(context.TODO(), map[string]rulespb.RuleGroupList{
		"user1": {&g1, &g2},
		"user2": {&g1},
	})
	gm, err := reg.Gather()
	require.NoError(t, err)
	mfm, err := util.NewMetricFamilyMap(gm)
	require.NoError(t, err)
	require.Equal(t, 3, len(mfm["cortex_ruler_backup_rule_group"].Metric))
	requireMetricEqual(t, mfm["cortex_ruler_backup_rule_group"].Metric[0], map[string]string{
		"user":       "user1",
		"rule_group": getGroupName(&g1, "user1"),
	}, float64(1))
	requireMetricEqual(t, mfm["cortex_ruler_backup_rule_group"].Metric[1], map[string]string{
		"user":       "user1",
		"rule_group": getGroupName(&g2, "user1"),
	}, float64(1))
	requireMetricEqual(t, mfm["cortex_ruler_backup_rule_group"].Metric[2], map[string]string{
		"user":       "user2",
		"rule_group": getGroupName(&g1, "user2"),
	}, float64(1))

	manager.setRuleGroups(context.TODO(), map[string]rulespb.RuleGroupList{
		"user1": {&g2Updated},
	})
	gm, err = reg.Gather()
	require.NoError(t, err)
	mfm, err = util.NewMetricFamilyMap(gm)
	require.NoError(t, err)
	require.Equal(t, 1, len(mfm["cortex_ruler_backup_rule_group"].Metric))
	requireMetricEqual(t, mfm["cortex_ruler_backup_rule_group"].Metric[0], map[string]string{
		"user":       "user1",
		"rule_group": getGroupName(&g2, "user1"),
	}, float64(1))
}

func requireMetricEqual(t *testing.T, m *io_prometheus_client.Metric, labels map[string]string, value float64) {
	l := m.GetLabel()
	require.Equal(t, len(labels), len(l))
	for _, pair := range l {
		require.Equal(t, labels[*pair.Name], *pair.Value)
	}
	require.Equal(t, value, *m.Gauge.Value)
}
