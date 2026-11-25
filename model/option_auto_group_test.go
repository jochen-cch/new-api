package model

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestValidateOptionValueRejectsInvalidMaxTokenAutoGroups(t *testing.T) {
	for _, value := range []string{"", "0", "-1", "1.5", "invalid"} {
		t.Run(value, func(t *testing.T) {
			assert.Error(t, validateOptionValue("MaxTokenAutoGroups", value))
		})
	}
	require.NoError(t, validateOptionValue("MaxTokenAutoGroups", "999999"))
}

func TestValidateOptionValueRejectsSystemTaskRetentionBelowOneDay(t *testing.T) {
	for _, value := range []string{"", "0", "-1", "1.5", "invalid"} {
		t.Run(value, func(t *testing.T) {
			assert.Error(t, validateOptionValue("system_task_setting.retention_days", value))
		})
	}
	require.NoError(t, validateOptionValue("system_task_setting.retention_days", "1"))
	require.NoError(t, validateOptionValue("system_task_setting.retention_days", "30"))
}

func TestValidateOptionValueChecksModelRatioOverrides(t *testing.T) {
	for _, key := range []string{"group_ratio_setting.user_group_model_ratio", "group_ratio_setting.group_model_ratio"} {
		t.Run(key, func(t *testing.T) {
			require.NoError(t, validateOptionValue(key, `{}`))
			require.NoError(t, validateOptionValue(key, `{"vip":{"gpt-4":0}}`))
			assert.Error(t, validateOptionValue(key, `{"vip":{"gpt-4":-1}}`))
			assert.Error(t, validateOptionValue(key, `{"":{"gpt-4":1}}`))
			assert.Error(t, validateOptionValue(key, `{"vip":1}`))
			assert.Error(t, validateOptionValue(key, ``))
		})
	}
	t.Run("group_ratio_setting.user_model_ratio", func(t *testing.T) {
		require.NoError(t, validateOptionValue("group_ratio_setting.user_model_ratio", `{"7":{"gpt-4":0.5}}`))
		assert.Error(t, validateOptionValue("group_ratio_setting.user_model_ratio", `{"vip":{"gpt-4":0.5}}`))
		assert.Error(t, validateOptionValue("group_ratio_setting.user_model_ratio", `{"0":{"gpt-4":0.5}}`))
	})
}

// The override tables are stored as one option row, so the matrix below checks
// the shape a deployment is expected to reach: 20 customers x 10 models.
const (
	modelRatioSubjects = 20
	modelRatioModels   = 10
)

// buildUserModelRatioJSON returns a user ID -> model -> ratio map whose ratio
// value identifies the subject, which lets the test prove which rule survived
// the database round trip.
func buildUserModelRatioJSON(t *testing.T) string {
	t.Helper()
	rules := make(map[string]map[string]float64, modelRatioSubjects)
	for uid := 1; uid <= modelRatioSubjects; uid++ {
		inner := make(map[string]float64, modelRatioModels)
		for m := 1; m <= modelRatioModels; m++ {
			inner[fmt.Sprintf("model-%02d", m)] = float64(uid) / 100
		}
		rules[strconv.Itoa(uid)] = inner
	}
	encoded, err := common.Marshal(rules)
	require.NoError(t, err)
	return string(encoded)
}

// TestUserModelRatioOptionRoundTripsOnEveryDialect stores the full override map
// the way UpdateOption persists it, then reads it back the way a starting node
// loads options. MySQL and PostgreSQL variants are skipped unless
// TEST_MYSQL_DSN / TEST_POSTGRES_DSN are configured, matching the convention of
// the other dialect matrices in this package.
func TestUserModelRatioOptionRoundTripsOnEveryDialect(t *testing.T) {
	const key = "group_ratio_setting.user_model_ratio"
	value := buildUserModelRatioJSON(t)
	require.NoError(t, validateOptionValue(key, value))

	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var driver gorm.Dialector
			switch dialect {
			case "sqlite":
				driver = sqlite.Open(filepath.Join(t.TempDir(), "option_matrix.db"))
			case "mysql":
				dsn := strings.TrimSpace(os.Getenv("TEST_MYSQL_DSN"))
				if dsn == "" {
					t.Skip("TEST_MYSQL_DSN is not configured")
				}
				driver = mysql.Open(dsn)
			case "postgres":
				dsn := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
				if dsn == "" {
					t.Skip("TEST_POSTGRES_DSN is not configured")
				}
				driver = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
			}
			db, err := gorm.Open(driver, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: "model_ratio_option_"}})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			// Each connection to a temporary SQLite file is the same database, but
			// keep one connection so concurrent writes cannot deadlock it.
			sqlDB.SetMaxOpenConns(1)
			previousDB, previousOptionMap := DB, common.OptionMap
			DB = db
			common.OptionMap = map[string]string{}
			t.Cleanup(func() {
				require.NoError(t, updateOptionMap(key, "{}"))
				DB = previousDB
				common.OptionMap = previousOptionMap
				require.NoError(t, db.Migrator().DropTable(&Option{}))
				require.NoError(t, sqlDB.Close())
			})

			// First boot creates the table, the two repeats must be no-ops.
			require.NoError(t, db.AutoMigrate(&Option{}))
			var version string
			if dialect == "sqlite" {
				require.NoError(t, db.Raw("select sqlite_version()").Scan(&version).Error)
			} else {
				require.NoError(t, db.Raw("select version()").Scan(&version).Error)
			}
			t.Logf("%s version: %s, %d rules in %d bytes", dialect, version, modelRatioSubjects*modelRatioModels, len(value))
			require.NoError(t, db.AutoMigrate(&Option{}))
			require.NoError(t, db.AutoMigrate(&Option{}))

			option := Option{Key: key}
			require.NoError(t, db.FirstOrCreate(&option, Option{Key: key}).Error)
			option.Value = value
			require.NoError(t, db.Save(&option).Error)

			var stored Option
			require.NoError(t, db.Where(&Option{Key: key}).First(&stored).Error)
			assert.Equal(t, value, stored.Value)

			var parsed map[string]map[string]float64
			require.NoError(t, common.UnmarshalJsonStr(stored.Value, &parsed))
			require.Len(t, parsed, modelRatioSubjects)
			counted := 0
			for _, inner := range parsed {
				counted += len(inner)
			}
			assert.Equal(t, modelRatioSubjects*modelRatioModels, counted)

			// Loading the row must register every rule, including the last one.
			require.NoError(t, updateOptionMap(stored.Key, stored.Value))
			info := ratio_setting.ResolveGroupRatio(ratio_setting.GroupRatioInput{
				UserId:     modelRatioSubjects,
				UserGroup:  "default",
				UsingGroup: "default",
				Model:      fmt.Sprintf("model-%02d", modelRatioModels),
			})
			assert.InDelta(t, float64(modelRatioSubjects)/100, info.GroupRatio, 1e-9)
			assert.Equal(t, "user_model_ratio", info.ModelRatioSource)
		})
	}
}
