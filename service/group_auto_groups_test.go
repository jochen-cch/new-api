package service

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func configureRequestAutoGroupsTest(t *testing.T) {
	t.Helper()
	originalMax := setting.GetMaxTokenAutoGroups()
	originalAutoGroups := setting.AutoGroups2JsonString()
	originalUsableGroups := setting.UserUsableGroups2JSONString()
	originalRatios := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, setting.UpdateMaxTokenAutoGroups("2"))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["vip","default","svip"]`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP","svip":"SVIP"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"svip":1}`))
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateMaxTokenAutoGroups(fmt.Sprintf("%d", originalMax)))
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatios))
	})
}

func newRequestAutoGroupsContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	return ctx
}

func TestGetRequestAutoGroupsInheritedListIsNotLimited(t *testing.T) {
	configureRequestAutoGroupsTest(t)
	ctx := newRequestAutoGroupsContext()

	groups := GetRequestAutoGroups(ctx, "default")

	assert.Equal(t, []string{"vip", "default", "svip"}, groups)
}

func TestGetRequestAutoGroupsFiltersBeforeApplyingCurrentLimit(t *testing.T) {
	configureRequestAutoGroupsTest(t)
	ctx := newRequestAutoGroupsContext()
	common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroups, []string{"revoked", "vip", "default", "svip"})
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`[]`))

	groups := GetRequestAutoGroups(ctx, "default")

	assert.Equal(t, []string{"vip", "default"}, groups)
	require.NoError(t, setting.UpdateMaxTokenAutoGroups("1"))
	assert.Equal(t, []string{"vip"}, GetRequestAutoGroups(ctx, "default"))
}

func TestGetRequestAutoGroupsDoesNotFallBackAfterPermissionChange(t *testing.T) {
	configureRequestAutoGroupsTest(t)
	ctx := newRequestAutoGroupsContext()
	common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroups, []string{"vip"})
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default"}`))

	groups := GetRequestAutoGroups(ctx, "default")

	assert.Empty(t, groups)
}

func TestGetRequesterModelGroupRatio(t *testing.T) {
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() { require.NoError(t, config.GlobalConfig.LoadFromDB(saved)) })
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"group_ratio_setting.group_ratio":            `{"default":1,"vip":2,"svip":3}`,
		"group_ratio_setting.group_group_ratio":      `{"employee":{"vip":1.5}}`,
		"group_ratio_setting.group_model_ratio":      `{"vip":{"gpt-4":0.8,"*":0.9}}`,
		"group_ratio_setting.user_group_model_ratio": `{}`,
		"group_ratio_setting.user_model_ratio":       `{"7":{"gpt-4":0.25},"9":{"*":0.05}}`,
	}))
	usableGroup := map[string]string{"default": "Default", "vip": "VIP"}

	t.Run("only the requester own rules are returned", func(t *testing.T) {
		assert.Equal(t, map[string]map[string]float64{
			"default": {"gpt-4": 0.25},
			"vip":     {"*": 0.9, "gpt-4": 0.25},
		}, GetRequesterModelGroupRatio(7, "member", usableGroup))
	})

	t.Run("the group ratio is the comparison base", func(t *testing.T) {
		assert.Equal(t, map[string]map[string]float64{
			"vip": {"*": 0.9, "gpt-4": 0.8},
		}, GetRequesterModelGroupRatio(8, "employee", usableGroup))
	})

	t.Run("a user wildcard shadows the group entries like billing does", func(t *testing.T) {
		assert.Equal(t, map[string]map[string]float64{
			"default": {"*": 0.05},
			"vip":     {"*": 0.05, "gpt-4": 0.05},
		}, GetRequesterModelGroupRatio(9, "member", usableGroup))
	})

	t.Run("entries equal to the base ratio are omitted", func(t *testing.T) {
		assert.Empty(t, GetRequesterModelGroupRatio(8, "member", map[string]string{"default": "Default"}))
	})
}
