package ratio_setting

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const groupRatioFixture = `{
	"group_ratio": {"default": 1, "vip": 2, "svip": 3},
	"group_group_ratio": {"employee": {"default": 0.5}},
	"user_model_ratio": {"7": {"gpt-4": 0.1}, "9": {"*": 0.05}, "10": {"free-model": 0}},
	"user_group_model_ratio": {"employee": {"gpt-4o": 0.3}},
	"group_model_ratio": {"svip": {"gpt-4": 0.2, "*": 0.9}}
}`

// loadGroupRatioFixture installs the option rows used by these tests and restores
// the previous values, because the maps keep their content across test cases.
func loadGroupRatioFixture(t *testing.T, jsonStr string) {
	t.Helper()
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		if strings.HasPrefix(key, "group_ratio_setting.") {
			saved[key] = value
		}
		return nil
	}))
	t.Cleanup(func() { require.NoError(t, config.GlobalConfig.LoadFromDB(saved)) })

	options := map[string]string{}
	for _, field := range []string{"group_ratio", "group_group_ratio", "group_special_usable_group", "user_model_ratio", "user_group_model_ratio", "group_model_ratio"} {
		options["group_ratio_setting."+field] = `{}`
	}
	var setting map[string]json.RawMessage
	require.NoError(t, common.UnmarshalJsonStr(jsonStr, &setting))
	require.NoError(t, config.GlobalConfig.LoadFromDB(options))
	for field, value := range setting {
		options["group_ratio_setting."+field] = string(value)
	}
	require.NoError(t, config.GlobalConfig.LoadFromDB(options))
}

func TestResolveGroupRatioPriorityChain(t *testing.T) {
	loadGroupRatioFixture(t, groupRatioFixture)

	cases := []struct {
		name        string
		input       GroupRatioInput
		wantRatio   float64
		wantSource  string
		wantSpecial bool
	}{
		{
			name:        "user model ratio beats every lower level",
			input:       GroupRatioInput{UserId: 7, UserGroup: "employee", UsingGroup: "svip", Model: "gpt-4"},
			wantRatio:   0.1,
			wantSource:  ModelRatioSourceUser,
			wantSpecial: true,
		},
		{
			name:        "user group model ratio beats using group model ratio",
			input:       GroupRatioInput{UserId: 8, UserGroup: "employee", UsingGroup: "svip", Model: "gpt-4o"},
			wantRatio:   0.3,
			wantSource:  ModelRatioSourceUserGr,
			wantSpecial: true,
		},
		{
			name:        "using group model ratio is the last model level",
			input:       GroupRatioInput{UserId: 8, UserGroup: "member", UsingGroup: "svip", Model: "gpt-4"},
			wantRatio:   0.2,
			wantSource:  ModelRatioSourceGroup,
			wantSpecial: true,
		},
		{
			name:        "inter group ratio still applies when no model rule matches",
			input:       GroupRatioInput{UserId: 8, UserGroup: "employee", UsingGroup: "default", Model: "claude"},
			wantRatio:   0.5,
			wantSource:  "",
			wantSpecial: true,
		},
		{
			name:        "plain group ratio is the fallback",
			input:       GroupRatioInput{UserId: 8, UserGroup: "member", UsingGroup: "vip", Model: "claude"},
			wantRatio:   2,
			wantSource:  "",
			wantSpecial: false,
		},
		{
			name:        "wildcard applies when the model has no exact entry",
			input:       GroupRatioInput{UserId: 8, UserGroup: "member", UsingGroup: "svip", Model: "gpt-4o"},
			wantRatio:   0.9,
			wantSource:  ModelRatioSourceGroup,
			wantSpecial: true,
		},
		{
			name:        "user wildcard beats the using group exact entry",
			input:       GroupRatioInput{UserId: 9, UserGroup: "member", UsingGroup: "svip", Model: "gpt-4"},
			wantRatio:   0.05,
			wantSource:  ModelRatioSourceUser,
			wantSpecial: true,
		},
		{
			name:        "zero is a valid discount",
			input:       GroupRatioInput{UserId: 10, UserGroup: "member", UsingGroup: "svip", Model: "free-model"},
			wantRatio:   0,
			wantSource:  ModelRatioSourceUser,
			wantSpecial: true,
		},
		{
			name:        "an empty model name never matches a model level rule",
			input:       GroupRatioInput{UserId: 7, UserGroup: "employee", UsingGroup: "default", Model: ""},
			wantRatio:   0.5,
			wantSource:  "",
			wantSpecial: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info := ResolveGroupRatio(tc.input)
			assert.InDelta(t, tc.wantRatio, info.GroupRatio, 1e-9)
			assert.Equal(t, tc.wantSource, info.ModelRatioSource)
			assert.Equal(t, tc.wantSpecial, info.HasSpecialRatio)
			if tc.wantSource != "" {
				assert.InDelta(t, tc.wantRatio, info.GroupSpecialRatio, 1e-9)
			}
		})
	}
}

func TestResolveGroupRatioUnknownGroupFallsBackToOne(t *testing.T) {
	loadGroupRatioFixture(t, groupRatioFixture)
	info := ResolveGroupRatio(GroupRatioInput{UserId: 8, UserGroup: "member", UsingGroup: "not-configured", Model: "gpt-4"})
	assert.Equal(t, 1.0, info.GroupRatio)
	assert.False(t, info.HasSpecialRatio)
	assert.Empty(t, info.ModelRatioSource)
}

func TestResolveGroupRatioIgnoresUnusableStoredRatio(t *testing.T) {
	loadGroupRatioFixture(t, groupRatioFixture)
	// The option validator rejects those values, but a partially written in-memory
	// map must not turn into an infinite or negative charge either.
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"group_ratio_setting.user_model_ratio": `{"11": {"gpt-4": -1, "*": 0.4}}`}))
	info := ResolveGroupRatio(GroupRatioInput{UserId: 11, UserGroup: "member", UsingGroup: "svip", Model: "gpt-4"})
	assert.InDelta(t, 0.4, info.GroupRatio, 1e-9)
	assert.Equal(t, ModelRatioSourceUser, info.ModelRatioSource)

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"group_ratio_setting.user_model_ratio": `{"11": {"gpt-4": -1}}`}))
	info = ResolveGroupRatio(GroupRatioInput{UserId: 11, UserGroup: "member", UsingGroup: "svip", Model: "gpt-4"})
	assert.InDelta(t, 0.2, info.GroupRatio, 1e-9)
	assert.Equal(t, ModelRatioSourceGroup, info.ModelRatioSource)
}

func TestCollectModelRatioKeys(t *testing.T) {
	loadGroupRatioFixture(t, groupRatioFixture)

	assert.Equal(t, []string{"*", "gpt-4", "gpt-4o"}, CollectModelRatioKeys(7, "employee", "svip"))
	assert.Equal(t, []string{"*", "gpt-4"}, CollectModelRatioKeys(8, "member", "svip"))
	assert.Empty(t, CollectModelRatioKeys(8, "member", "vip"), "groups without any rule produce no keys")
}

func TestCheckModelRatioMap(t *testing.T) {
	t.Run("group subjects", func(t *testing.T) {
		require.NoError(t, CheckModelRatioMap(`{}`, false))
		require.NoError(t, CheckModelRatioMap(`{"vip":{"gpt-4":0}}`, false))
		require.NoError(t, CheckModelRatioMap(`{"vip":{"*":1.5}}`, false))
	})
	t.Run("user subjects", func(t *testing.T) {
		require.NoError(t, CheckModelRatioMap(`{"7":{"gpt-4":0.1}}`, true))
	})
	for _, tc := range []struct {
		name    string
		jsonStr string
		uidKeys bool
	}{
		{name: "negative ratio", jsonStr: `{"vip":{"gpt-4":-0.1}}`},
		{name: "infinite ratio", jsonStr: `{"vip":{"gpt-4":1e999}}`},
		{name: "empty group", jsonStr: `{"":{"gpt-4":1}}`},
		{name: "blank group", jsonStr: `{"   ":{"gpt-4":1}}`},
		{name: "empty model", jsonStr: `{"vip":{"":1}}`},
		{name: "not an object", jsonStr: `[1,2]`},
		{name: "ratio is not a number", jsonStr: `{"vip":{"gpt-4":"0.1"}}`},
		{name: "user key is not an id", jsonStr: `{"vip":{"gpt-4":0.1}}`, uidKeys: true},
		{name: "user id is not positive", jsonStr: `{"0":{"gpt-4":0.1}}`, uidKeys: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Error(t, CheckModelRatioMap(tc.jsonStr, tc.uidKeys))
		})
	}
	require.NoError(t, CheckModelRatioMap(`{"7":{"gpt-4":0.1}}`, true))
	assert.Error(t, CheckModelRatioMap(`{"vip":{"gpt-4":0.1}}`, true), "a group key is not a user id")
}

func TestCheckModelRatioMapRejectsNaNThroughFloatBounds(t *testing.T) {
	// JSON has no NaN literal, so the guard is exercised on the value helper and on
	// the overflow path, which encoding/json reports as an out of range number.
	assert.False(t, isUsableGroupRatio(math.NaN()))
	assert.False(t, isUsableGroupRatio(math.Inf(-1)))
	assert.False(t, isUsableGroupRatio(-0.5))
	assert.True(t, isUsableGroupRatio(0))
	assert.NoError(t, CheckModelRatioMap(`{"vip":{"gpt-4":0}}`, false))
}
