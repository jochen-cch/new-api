package ratio_setting

import (
	"encoding/json"
	"errors"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/types"
)

var defaultGroupRatio = map[string]float64{
	"default": 1,
	"vip":     1,
	"svip":    1,
}

var groupRatioMap = types.NewRWMap[string, float64]()

var defaultGroupGroupRatio = map[string]map[string]float64{
	"vip": {
		"edit_this": 0.9,
	},
}

var groupGroupRatioMap = types.NewRWMap[string, map[string]float64]()

var defaultGroupSpecialUsableGroup = map[string]map[string]string{}

// Model level ratio overrides replace the group ratio of the request instead
// of multiplying it, so the value is the effective group ratio for that model.
// The "*" model key is the fallback used when the exact model has no entry.
const (
	ModelRatioWildcard     = "*"
	ModelRatioSourceUser   = "user_model_ratio"
	ModelRatioSourceUserGr = "user_group_model_ratio"
	ModelRatioSourceGroup  = "group_model_ratio"
)

var (
	// defaultUserModelRatio is an example of the shape kept in the option row;
	// nothing is enabled out of the box.
	defaultUserModelRatio          = map[int]map[string]float64{}
	defaultUserGroupModelRatio     = map[string]map[string]float64{}
	defaultUsingGroupModelRatio    = map[string]map[string]float64{}
	userModelRatioMap              = types.NewRWMap[int, map[string]float64]()
	userGroupModelRatioMap         = types.NewRWMap[string, map[string]float64]()
	usingGroupModelRatioMap        = types.NewRWMap[string, map[string]float64]()
)

type GroupRatioSetting struct {
	GroupRatio              *types.RWMap[string, float64]              `json:"group_ratio"`
	GroupGroupRatio         *types.RWMap[string, map[string]float64]   `json:"group_group_ratio"`
	GroupSpecialUsableGroup *types.RWMap[string, map[string]string]    `json:"group_special_usable_group"`
	UserModelRatio          *types.RWMap[int, map[string]float64]      `json:"user_model_ratio"`
	UserGroupModelRatio     *types.RWMap[string, map[string]float64]   `json:"user_group_model_ratio"`
	UsingGroupModelRatio    *types.RWMap[string, map[string]float64]   `json:"group_model_ratio"`
}

var groupRatioSetting GroupRatioSetting

func init() {
	groupSpecialUsableGroup := types.NewRWMap[string, map[string]string]()
	groupSpecialUsableGroup.AddAll(defaultGroupSpecialUsableGroup)

	groupRatioMap.AddAll(defaultGroupRatio)
	groupGroupRatioMap.AddAll(defaultGroupGroupRatio)
	userModelRatioMap.AddAll(defaultUserModelRatio)
	userGroupModelRatioMap.AddAll(defaultUserGroupModelRatio)
	usingGroupModelRatioMap.AddAll(defaultUsingGroupModelRatio)

	groupRatioSetting = GroupRatioSetting{
		GroupSpecialUsableGroup: groupSpecialUsableGroup,
		GroupRatio:              groupRatioMap,
		GroupGroupRatio:         groupGroupRatioMap,
		UserModelRatio:          userModelRatioMap,
		UserGroupModelRatio:     userGroupModelRatioMap,
		UsingGroupModelRatio:    usingGroupModelRatioMap,
	}

	config.GlobalConfig.Register("group_ratio_setting", &groupRatioSetting)
}

func GetGroupRatioSetting() *GroupRatioSetting {
	if groupRatioSetting.GroupSpecialUsableGroup == nil {
		groupRatioSetting.GroupSpecialUsableGroup = types.NewRWMap[string, map[string]string]()
		groupRatioSetting.GroupSpecialUsableGroup.AddAll(defaultGroupSpecialUsableGroup)
	}
	return &groupRatioSetting
}

func GetGroupRatioCopy() map[string]float64 {
	return groupRatioMap.ReadAll()
}

func ContainsGroupRatio(name string) bool {
	_, ok := groupRatioMap.Get(name)
	return ok
}

func GroupRatio2JSONString() string {
	return groupRatioMap.MarshalJSONString()
}

func UpdateGroupRatioByJSONString(jsonStr string) error {
	return types.LoadFromJsonString(groupRatioMap, jsonStr)
}

func GetGroupRatio(name string) float64 {
	ratio, ok := groupRatioMap.Get(name)
	if !ok {
		common.SysLog(common.LogText("group ratio not found: %s", name))
		return 1
	}
	return ratio
}

func GetGroupGroupRatio(userGroup, usingGroup string) (float64, bool) {
	gp, ok := groupGroupRatioMap.Get(userGroup)
	if !ok {
		return -1, false
	}
	ratio, ok := gp[usingGroup]
	if !ok {
		return -1, false
	}
	return ratio, true
}

func GroupGroupRatio2JSONString() string {
	return groupGroupRatioMap.MarshalJSONString()
}

func UpdateGroupGroupRatioByJSONString(jsonStr string) error {
	return types.LoadFromJsonString(groupGroupRatioMap, jsonStr)
}

// GroupRatioInput identifies the billing subject whose effective group ratio is
// being resolved. Model must be the same billing model name the price lookup
// uses, so a discount is keyed by the model the request is actually priced with.
type GroupRatioInput struct {
	UserId     int
	UserGroup  string
	UsingGroup string
	Model      string
}

// ResolveGroupRatio returns the effective group ratio for a request. Model level
// overrides are checked from the most specific identity to the least, then the
// inter-group special ratio, then the plain group ratio. A matched override
// replaces the group ratio rather than multiplying it.
func ResolveGroupRatio(in GroupRatioInput) types.GroupRatioInfo {
	if ratio, source, ok := lookupModelGroupRatio(in); ok {
		return types.GroupRatioInfo{
			GroupRatio:        ratio,
			GroupSpecialRatio: ratio,
			HasSpecialRatio:   true,
			ModelRatioSource:  source,
		}
	}
	if specialRatio, ok := GetGroupGroupRatio(in.UserGroup, in.UsingGroup); ok {
		return types.GroupRatioInfo{
			GroupRatio:        specialRatio,
			GroupSpecialRatio: specialRatio,
			HasSpecialRatio:   true,
		}
	}
	return types.GroupRatioInfo{
		GroupRatio:        GetGroupRatio(in.UsingGroup),
		GroupSpecialRatio: -1,
	}
}

// lookupModelGroupRatio walks the three model level tables in priority order.
func lookupModelGroupRatio(in GroupRatioInput) (float64, string, bool) {
	if in.Model == "" {
		return 0, "", false
	}
	if ratios, ok := userModelRatioMap.Get(in.UserId); ok {
		if ratio, ok := lookupModelRatio(ratios, in.Model); ok {
			return ratio, ModelRatioSourceUser, true
		}
	}
	if ratios, ok := userGroupModelRatioMap.Get(in.UserGroup); ok {
		if ratio, ok := lookupModelRatio(ratios, in.Model); ok {
			return ratio, ModelRatioSourceUserGr, true
		}
	}
	if ratios, ok := usingGroupModelRatioMap.Get(in.UsingGroup); ok {
		if ratio, ok := lookupModelRatio(ratios, in.Model); ok {
			return ratio, ModelRatioSourceGroup, true
		}
	}
	return 0, "", false
}

// lookupModelRatio prefers the exact model name over the wildcard entry and
// ignores stored values that cannot be a safe multiplier.
func lookupModelRatio(ratios map[string]float64, model string) (float64, bool) {
	if ratio, ok := ratios[model]; ok {
		if isUsableGroupRatio(ratio) {
			return ratio, true
		}
	}
	ratio, ok := ratios[ModelRatioWildcard]
	if ok && isUsableGroupRatio(ratio) {
		return ratio, true
	}
	return 0, false
}

func isUsableGroupRatio(ratio float64) bool {
	return ratio >= 0 && !math.IsNaN(ratio) && !math.IsInf(ratio, 0)
}

// CollectModelRatioKeys returns every model key the three override tables
// address for one identity, sorted and deduplicated. The caller resolves each
// key with ResolveGroupRatio so wildcard precedence stays identical to billing.
func CollectModelRatioKeys(userId int, userGroup, usingGroup string) []string {
	userRatios, _ := userModelRatioMap.Get(userId)
	userGroupRatios, _ := userGroupModelRatioMap.Get(userGroup)
	usingGroupRatios, _ := usingGroupModelRatioMap.Get(usingGroup)
	seen := make(map[string]struct{}, len(userRatios)+len(userGroupRatios)+len(usingGroupRatios))
	for _, ratios := range []map[string]float64{userRatios, userGroupRatios, usingGroupRatios} {
		for model := range ratios {
			seen[model] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return nil
	}
	keys := make([]string, 0, len(seen))
	for model := range seen {
		keys = append(keys, model)
	}
	slices.Sort(keys)
	return keys
}

// CheckModelRatioMap validates a subject -> model -> ratio mapping before it is
// persisted. When userSubjects is true the outer keys must be positive user ids.
func CheckModelRatioMap(jsonStr string, userSubjects bool) error {
	check := make(map[string]map[string]float64)
	if err := common.UnmarshalJsonStr(jsonStr, &check); err != nil {
		return err
	}
	for subject, ratios := range check {
		if userSubjects {
			userId, err := strconv.Atoi(subject)
			if err != nil || userId <= 0 {
				return errors.New("model ratio key must be a user id: " + subject)
			}
		} else if strings.TrimSpace(subject) == "" {
			return errors.New("model ratio group cannot be empty")
		}
		for model, ratio := range ratios {
			if strings.TrimSpace(model) == "" {
				return errors.New("model ratio model cannot be empty: " + subject)
			}
			if !isUsableGroupRatio(ratio) {
				return errors.New("model ratio must be a finite number not less than 0: " + subject + "/" + model)
			}
		}
	}
	return nil
}

func CheckGroupRatio(jsonStr string) error {
	checkGroupRatio := make(map[string]float64)
	err := json.Unmarshal([]byte(jsonStr), &checkGroupRatio)
	if err != nil {
		return err
	}
	for name, ratio := range checkGroupRatio {
		if ratio < 0 {
			return errors.New("group ratio must be not less than 0: " + name)
		}
	}
	return nil
}
