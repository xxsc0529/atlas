// Copyright 2021-present The Atlas Authors. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package mysql

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"ariga.io/atlas/sql/internal/sqltest"
	"ariga.io/atlas/sql/internal/sqlx"
	"ariga.io/atlas/sql/schema"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func Test_isOBFullTextInternalCol(t *testing.T) {
	require.True(t, isOBFullTextInternalCol("__doc_id_1780646353769622"))
	require.True(t, isOBFullTextInternalCol("__word_segment_28_1780646353769652"))
	require.False(t, isOBFullTextInternalCol("title"))
	require.False(t, isOBFullTextInternalCol("__other"))
}

func Test_fullTextIndexColumns(t *testing.T) {
	create := "CREATE TABLE `it_faq` (" +
		"`id` bigint NOT NULL," +
		"FULLTEXT KEY `ft_title` (`title`) WITH PARSER space," +
		"FULLTEXT INDEX `ft_both` (`title`, `content`)" +
		")"
	cols, err := fullTextIndexColumns(create, "ft_title")
	require.NoError(t, err)
	require.Equal(t, []string{"title"}, cols)
	cols, err = fullTextIndexColumns(create, "ft_both")
	require.NoError(t, err)
	require.Equal(t, []string{"title", "content"}, cols)
	_, err = fullTextIndexColumns(create, "missing")
	require.Error(t, err)
}

func TestOceanBase_FullTextInspect(t *testing.T) {
	db, m, err := sqlmock.New()
	require.NoError(t, err)
	mk := mock{m}
	mk.version("5.7.25-OceanBase-v4.3.5.6")
	mk.ExpectQuery(sqltest.Escape(fmt.Sprintf(schemasQueryArgs, "= ?"))).
		WithArgs("public").
		WillReturnRows(sqltest.Rows(`
+-------------+----------------------------+------------------------+
| SCHEMA_NAME | DEFAULT_CHARACTER_SET_NAME | DEFAULT_COLLATION_NAME |
+-------------+----------------------------+------------------------+
| public      | utf8mb4                    | utf8mb4_unicode_ci     |
+-------------+----------------------------+------------------------+
`))
	mk.tableExists("public", "it_faq", true)
	mk.ExpectQuery(queryColumns).
		WithArgs("public", "it_faq").
		WillReturnRows(sqltest.Rows(`
+------------+-------------+--------------+----------------+-------------+------------+----------------+----------------+--------------------+----------------+---------------------------+
| TABLE_NAME | COLUMN_NAME | COLUMN_TYPE  | COLUMN_COMMENT | IS_NULLABLE | COLUMN_KEY | COLUMN_DEFAULT | EXTRA          | CHARACTER_SET_NAME | COLLATION_NAME | GENERATION_EXPRESSION     |
+------------+-------------+--------------+----------------+-------------+------------+----------------+----------------+--------------------+----------------+---------------------------+
| it_faq     | id          | bigint(20)   |                | NO          | PRI        | NULL           |                | NULL               | NULL           | NULL                      |
| it_faq     | title       | varchar(255) |                | YES         |            | NULL           |                | utf8mb4            | utf8mb4_bin    | NULL                      |
| it_faq     | content     | text         |                | YES         |            | NULL           |                | utf8mb4            | utf8mb4_bin    | NULL                      |
+------------+-------------+--------------+----------------+-------------+------------+----------------+----------------+--------------------+----------------+---------------------------+
`))
	mk.ExpectQuery(queryIndexes).
		WithArgs("public", "it_faq").
		WillReturnRows(sqltest.Rows(`
+------------+------------+------------------------------------+------------+--------------+--------------+---------+--------------+------------+------------------+
| TABLE_NAME | INDEX_NAME | COLUMN_NAME                        | NON_UNIQUE | SEQ_IN_INDEX | INDEX_TYPE   | DESC    | COMMENT      | SUB_PART   | EXPRESSION       |
+------------+------------+------------------------------------+------------+--------------+--------------+---------+--------------+------------+------------------+
| it_faq     | ft_title   | __doc_id_1780646353769622          |          1 |            1 | FULLTEXT     | 0       |              |       NULL | NULL             |
| it_faq     | ft_title   | __word_segment_28_1780646353769652 |          1 |            2 | FULLTEXT     | 0       |              |       NULL | NULL             |
| it_faq     | ft_both    | __doc_id_1781506284092639          |          1 |            2 | FULLTEXT     | 0       |              |       NULL | NULL             |
| it_faq     | ft_both    | __word_segment_20_1781506284092949 |          1 |            1 | FULLTEXT     | 0       |              |       NULL | NULL             |
+------------+------------+------------------------------------+------------+--------------+--------------+---------+--------------+------------+------------------+
`))
	mk.noFKs()
	mk.ExpectQuery(sqltest.Escape("SHOW CREATE TABLE `public`.`it_faq`")).
		WillReturnRows(sqlmock.NewRows([]string{"Table", "Create Table"}).
			AddRow("it_faq", "CREATE TABLE `it_faq` (`id` bigint(20) NOT NULL AUTO_INCREMENT, `title` varchar(255) DEFAULT NULL, `content` text DEFAULT NULL, PRIMARY KEY (`id`), FULLTEXT KEY `ft_title` (`title`) WITH PARSER space, FULLTEXT KEY `ft_both` (`title`, `content`) WITH PARSER space) DEFAULT CHARSET=utf8mb4"))
	drv, err := Open(db)
	require.NoError(t, err)
	s, err := drv.InspectSchema(context.Background(), "public", nil)
	require.NoError(t, err)
	require.Len(t, s.Tables, 1)
	table := s.Tables[0]
	require.Equal(t, "it_faq", table.Name)
	ftTitle, ok := table.Index("ft_title")
	require.True(t, ok)
	require.True(t, sqlx.Has(ftTitle.Attrs, &IndexType{T: IndexTypeFullText}))
	require.Len(t, ftTitle.Parts, 1)
	require.Equal(t, "title", ftTitle.Parts[0].C.Name)
	ftBoth, ok := table.Index("ft_both")
	require.True(t, ok)
	require.Len(t, ftBoth.Parts, 2)
	require.Equal(t, "title", ftBoth.Parts[0].C.Name)
	require.Equal(t, "content", ftBoth.Parts[1].C.Name)
}

func TestOceanBase_DriverSelection(t *testing.T) {
	db, mk, err := sqlmock.New()
	require.NoError(t, err)
	mk.ExpectQuery("SELECT @@version, @@collation_server, @@character_set_server, @@lower_case_table_names").
		WillReturnRows(sqlmock.NewRows([]string{"version", "collation", "charset", "lcnames"}).
			AddRow("5.7.25-OceanBase-v3.2.3", "utf8mb4_general_ci", "utf8mb4", 0))
	drv, err := Open(db)
	require.NoError(t, err)
	d, ok := drv.(*Driver)
	require.True(t, ok, "expected *Driver")
	_, ok = d.PlanApplier.(*oplanApply)
	require.True(t, ok, "expected OceanBase planner to be selected for OceanBase version")
}

func TestOceanBase_NotSelectedForMySQL(t *testing.T) {
	db, mk, err := sqlmock.New()
	require.NoError(t, err)
	mk.ExpectQuery("SELECT @@version, @@collation_server, @@character_set_server, @@lower_case_table_names").
		WillReturnRows(sqlmock.NewRows([]string{"version", "collation", "charset", "lcnames"}).
			AddRow("8.0.28", "utf8mb4_general_ci", "utf8mb4", 0))
	drv, err := Open(db)
	require.NoError(t, err)
	d, ok := drv.(*Driver)
	require.True(t, ok, "expected *Driver")
	_, ok = d.PlanApplier.(*oplanApply)
	require.False(t, ok, "OceanBase planner should not be selected for standard MySQL")
	_, ok = d.PlanApplier.(*planApply)
	require.True(t, ok, "standard MySQL planner should be selected")
}

func TestOceanBase_NotSelectedForTiDB(t *testing.T) {
	db, mk, err := sqlmock.New()
	require.NoError(t, err)
	mk.ExpectQuery("SELECT @@version, @@collation_server, @@character_set_server, @@lower_case_table_names").
		WillReturnRows(sqlmock.NewRows([]string{"version", "collation", "charset", "lcnames"}).
			AddRow("5.7.25-TiDB-v5.4.0", "utf8mb4_general_ci", "utf8mb4", 0))
	drv, err := Open(db)
	require.NoError(t, err)
	d, ok := drv.(*Driver)
	require.True(t, ok, "expected *Driver")
	_, ok = d.PlanApplier.(*oplanApply)
	require.False(t, ok, "OceanBase planner should not be selected for TiDB")
	_, ok = d.PlanApplier.(*tplanApply)
	require.True(t, ok, "TiDB planner should be selected")
}

func TestOceanBase_Priority(t *testing.T) {
	tests := []struct {
		name     string
		change   schema.Change
		priority int
	}{
		{"DropForeignKey", &schema.DropForeignKey{F: &schema.ForeignKey{Symbol: "fk"}}, 1},
		{"DropIndex", &schema.DropIndex{I: &schema.Index{Name: "idx"}}, 1},
		{"DropAttr", &schema.DropAttr{A: &schema.Comment{}}, 1},
		{"DropCheck", &schema.DropCheck{C: &schema.Check{Name: "chk"}}, 1},
		{"DropColumn", &schema.DropColumn{C: &schema.Column{Name: "col"}}, 2},
		{"ModifyColumn", &schema.ModifyColumn{From: &schema.Column{Name: "a"}, To: &schema.Column{Name: "a"}}, 3},
		{"RenameColumn", &schema.RenameColumn{From: &schema.Column{Name: "a"}, To: &schema.Column{Name: "b"}}, 3},
		{"AddColumn", &schema.AddColumn{C: &schema.Column{Name: "col"}}, 4},
		{"ModifyIndex", &schema.ModifyIndex{From: &schema.Index{Name: "idx"}, To: &schema.Index{Name: "idx"}}, 5},
		{"ModifyForeignKey", &schema.ModifyForeignKey{From: &schema.ForeignKey{Symbol: "fk"}, To: &schema.ForeignKey{Symbol: "fk"}}, 5},
		{"AddIndex", &schema.AddIndex{I: &schema.Index{Name: "idx"}}, 6},
		{"AddForeignKey", &schema.AddForeignKey{F: &schema.ForeignKey{Symbol: "fk"}}, 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := opriority(tt.change); got != tt.priority {
				t.Errorf("opriority(%s) = %v, want %v", tt.name, got, tt.priority)
			}
		})
	}
}

func TestOceanBase_PriorityOrder(t *testing.T) {
	// Test that priorities are correctly ordered
	require.Less(t, opriority(&schema.DropForeignKey{}), opriority(&schema.DropColumn{}), "DropForeignKey should have higher priority than DropColumn")
	require.Less(t, opriority(&schema.DropColumn{}), opriority(&schema.ModifyColumn{}), "DropColumn should have higher priority than ModifyColumn")
	require.Less(t, opriority(&schema.ModifyColumn{}), opriority(&schema.AddColumn{}), "ModifyColumn should have higher priority than AddColumn")
	require.Less(t, opriority(&schema.AddColumn{}), opriority(&schema.AddIndex{}), "AddColumn should have higher priority than AddIndex")
	require.Less(t, opriority(&schema.AddIndex{}), opriority(&schema.AddForeignKey{}), "AddIndex should have higher priority than AddForeignKey")
}

func TestOceanBase_Flat(t *testing.T) {
	users := schema.NewTable("users")
	changes := []schema.Change{
		&schema.ModifyTable{
			T: users,
			Changes: []schema.Change{
				&schema.AddColumn{C: schema.NewColumn("a")},
				&schema.AddColumn{C: schema.NewColumn("b")},
				&schema.DropColumn{C: schema.NewColumn("c")},
			},
		},
		&schema.ModifyTable{
			T: users,
			Changes: []schema.Change{
				&schema.AddIndex{I: &schema.Index{Name: "idx"}},
			},
		},
	}
	flat := oflat(changes)
	require.Len(t, flat, 4, "expected 4 flattened changes")
	for i, c := range flat {
		mt, ok := c.(*schema.ModifyTable)
		require.True(t, ok, "change %d should be ModifyTable", i)
		require.Len(t, mt.Changes, 1, "change %d should have exactly 1 sub-change", i)
	}
}

func TestOceanBase_FlatPreservesOrder(t *testing.T) {
	users := schema.NewTable("users")
	changes := []schema.Change{
		&schema.ModifyTable{
			T: users,
			Changes: []schema.Change{
				&schema.AddColumn{C: schema.NewColumn("a")},
				&schema.DropColumn{C: schema.NewColumn("b")},
				&schema.AddIndex{I: &schema.Index{Name: "idx"}},
			},
		},
	}
	flat := oflat(changes)
	require.Len(t, flat, 3)
	// Verify the order is preserved
	require.IsType(t, &schema.AddColumn{}, flat[0].(*schema.ModifyTable).Changes[0])
	require.IsType(t, &schema.DropColumn{}, flat[1].(*schema.ModifyTable).Changes[0])
	require.IsType(t, &schema.AddIndex{}, flat[2].(*schema.ModifyTable).Changes[0])
}

func TestOceanBase_FlatModifySchema(t *testing.T) {
	s := schema.New("test")
	changes := []schema.Change{
		&schema.ModifySchema{
			S: s,
			Changes: []schema.Change{
				&schema.AddTable{T: schema.NewTable("users")},
				&schema.AddTable{T: schema.NewTable("posts")},
			},
		},
	}
	flat := oflat(changes)
	require.Len(t, flat, 2)
	for i, c := range flat {
		ms, ok := c.(*schema.ModifySchema)
		require.True(t, ok, "change %d should be ModifySchema", i)
		require.Len(t, ms.Changes, 1, "change %d should have exactly 1 sub-change", i)
	}
}

func TestOceanBase_PlanChanges(t *testing.T) {
	db, _, err := newMigrate("5.7.25-OceanBase-v3.2.3")
	require.NoError(t, err)

	nameCol := schema.NewStringColumn("name", "varchar(255)")
	users := schema.NewTable("users").
		AddColumns(
			schema.NewIntColumn("id", "int"),
			nameCol,
		).
		AddIndexes(
			schema.NewIndex("idx_name").
				AddParts(schema.NewColumnPart(nameCol)),
		)

	changes := []schema.Change{
		&schema.ModifyTable{
			T: users,
			Changes: []schema.Change{
				&schema.AddColumn{C: schema.NewStringColumn("email", "varchar(255)")},
				&schema.DropIndex{I: users.Indexes[0]},
				&schema.DropColumn{C: nameCol},
				&schema.AddIndex{I: schema.NewIndex("idx_email").
					AddParts(schema.NewColumnPart(schema.NewStringColumn("email", "varchar(255)")))},
			},
		},
	}

	plan, err := db.PlanChanges(context.Background(), "ob_plan", changes)
	require.NoError(t, err)
	// Each sub-change should produce a separate ALTER TABLE statement.
	require.Len(t, plan.Changes, 4)
	// Verify each statement is a single ALTER TABLE operation.
	for _, c := range plan.Changes {
		require.True(t, strings.HasPrefix(c.Cmd, "ALTER TABLE"), "expected ALTER TABLE, got: %s", c.Cmd)
	}
	// Verify ordering: DropIndex (priority 1) and DropColumn (priority 2)
	// should come before AddColumn (priority 4) and AddIndex (priority 6).
	require.Contains(t, plan.Changes[0].Cmd, "DROP INDEX")
	require.Contains(t, plan.Changes[1].Cmd, "DROP COLUMN")
	require.Contains(t, plan.Changes[2].Cmd, "ADD COLUMN")
	require.Contains(t, plan.Changes[3].Cmd, "ADD INDEX")
}

func TestOceanBase_PlanChangesSingleChange(t *testing.T) {
	db, _, err := newMigrate("5.7.25-OceanBase-v3.2.3")
	require.NoError(t, err)

	users := schema.NewTable("users").
		AddColumns(schema.NewIntColumn("id", "int"))

	changes := []schema.Change{
		&schema.ModifyTable{
			T: users,
			Changes: []schema.Change{
				&schema.AddColumn{C: schema.NewStringColumn("name", "varchar(255)")},
			},
		},
	}

	plan, err := db.PlanChanges(context.Background(), "ob_single", changes)
	require.NoError(t, err)
	require.Len(t, plan.Changes, 1)
	require.Contains(t, plan.Changes[0].Cmd, "ADD COLUMN")
}

func TestOceanBase_PlanChangesVsMySQL(t *testing.T) {
	// MySQL should combine changes into a single ALTER TABLE.
	mysqlDB, _, err := newMigrate("8.0.28")
	require.NoError(t, err)
	// OceanBase should split them.
	obDB, _, err := newMigrate("5.7.25-OceanBase-v3.2.3")
	require.NoError(t, err)

	users := schema.NewTable("users").
		AddColumns(
			schema.NewIntColumn("id", "int"),
			schema.NewStringColumn("name", "varchar(255)"),
		)

	changes := []schema.Change{
		&schema.ModifyTable{
			T: users,
			Changes: []schema.Change{
				&schema.AddColumn{C: schema.NewStringColumn("email", "varchar(255)")},
				&schema.AddColumn{C: schema.NewStringColumn("phone", "varchar(32)")},
			},
		},
	}

	mysqlPlan, err := mysqlDB.PlanChanges(context.Background(), "mysql_plan", changes)
	require.NoError(t, err)
	obPlan, err := obDB.PlanChanges(context.Background(), "ob_plan", changes)
	require.NoError(t, err)

	// MySQL: one combined ALTER TABLE.
	require.Len(t, mysqlPlan.Changes, 1)
	// OceanBase: one ALTER TABLE per change.
	require.Len(t, obPlan.Changes, 2)

	for _, c := range obPlan.Changes {
		// Each OceanBase ALTER should have exactly one ADD COLUMN.
		require.Equal(t, 1, strings.Count(c.Cmd, "ADD COLUMN"),
			"OceanBase ALTER should contain exactly one operation, got: %s", c.Cmd)
	}
}

func TestOceanBase_PlanChangesReversible(t *testing.T) {
	db, _, err := newMigrate("5.7.25-OceanBase-v3.2.3")
	require.NoError(t, err)

	users := schema.NewTable("users").
		AddColumns(schema.NewIntColumn("id", "int"))

	plan, err := db.PlanChanges(context.Background(), "ob_rev", []schema.Change{
		&schema.ModifyTable{
			T: users,
			Changes: []schema.Change{
				&schema.AddColumn{C: schema.NewStringColumn("a", "varchar(255)")},
				&schema.AddColumn{C: schema.NewStringColumn("b", "varchar(255)")},
			},
		},
	})
	require.NoError(t, err)
	require.True(t, plan.Reversible)
	for _, c := range plan.Changes {
		require.NotEmpty(t, c.Reverse, "each reversible change should have a reverse statement")
	}
}
