// Copyright 2021-present The Atlas Authors. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package mysql

import (
	"testing"

	"ariga.io/atlas/sql/schema"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

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