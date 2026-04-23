// Copyright 2021-present The Atlas Authors. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package mysql

import (
	"context"
	"sort"

	"ariga.io/atlas/sql/internal/sqlx"
	"ariga.io/atlas/sql/migrate"
	"ariga.io/atlas/sql/schema"
)

type (
	// oplanApply decorates MySQL planApply for OceanBase.
	oplanApply struct{ planApply }
	// odiff decorates MySQL diff for OceanBase.
	odiff struct{ diff }
	// oinspect decorates MySQL inspect for OceanBase.
	oinspect struct{ inspect }
)

// opriority computes the priority of each change for OceanBase.
//
// OceanBase has limitations when executing mixed online/offline DDL operations
// in a single ALTER TABLE statement. This function orders the changes to ensure
// correct execution sequence:
// 1. Drop constraints (foreign keys, indexes, checks, attributes) first
// 2. Drop columns
// 3. Modify/rename columns
// 4. Add columns
// 5. Modify indexes/foreign keys
// 6. Add indexes
// 7. Add foreign keys
// 8. Other changes
func opriority(change schema.Change) int {
	switch c := change.(type) {
	case *schema.ModifyTable:
		// Each ModifyTable should have a single change since we apply `oflat` before sorting.
		return opriority(c.Changes[0])
	case *schema.ModifySchema:
		// Each ModifySchema should have a single change since we apply `oflat` before sorting.
		return opriority(c.Changes[0])
	case *schema.DropForeignKey, *schema.DropIndex, *schema.DropAttr, *schema.DropCheck:
		return 1
	case *schema.DropColumn:
		return 2
	case *schema.ModifyColumn, *schema.RenameColumn:
		return 3
	case *schema.AddColumn:
		return 4
	case *schema.ModifyIndex, *schema.ModifyForeignKey:
		return 5
	case *schema.AddIndex:
		return 6
	case *schema.AddForeignKey:
		return 7
	default:
		return 8
	}
}

// oflat takes a list of changes and breaks them down to single atomic changes
// (e.g., no ModifyTable with multiple changes inside it). Note that, the only
// "changes" that include sub-changes are `ModifyTable` and `ModifySchema`.
func oflat(changes []schema.Change) []schema.Change {
	var flat []schema.Change
	for _, change := range changes {
		switch m := change.(type) {
		case *schema.ModifyTable:
			for _, c := range m.Changes {
				flat = append(flat, &schema.ModifyTable{
					T:       m.T,
					Changes: []schema.Change{c},
				})
			}
		case *schema.ModifySchema:
			for _, c := range m.Changes {
				flat = append(flat, &schema.ModifySchema{
					S:       m.S,
					Changes: []schema.Change{c},
				})
			}
		default:
			flat = append(flat, change)
		}
	}
	return flat
}

// PlanChanges returns a migration plan for the given schema changes.
// OceanBase requires each ALTER TABLE to contain only a single change,
// so we break down composite changes into individual statements.
func (p *oplanApply) PlanChanges(ctx context.Context, name string, changes []schema.Change, opts ...migrate.PlanOption) (*migrate.Plan, error) {
	planned, err := sqlx.DetachCycles(changes)
	if err != nil {
		return nil, err
	}
	planned = oflat(planned)
	sort.SliceStable(planned, func(i, j int) bool {
		return opriority(planned[i]) < opriority(planned[j])
	})
	s := &state{
		conn: p.conn,
		Plan: migrate.Plan{
			Name: name,
			// A plan is reversible, if all
			// its changes are reversible.
			Reversible:    true,
			Transactional: false,
		},
	}
	for _, c := range planned {
		// Use the planner of MySQL with each "atomic" change.
		plan, err := p.planApply.PlanChanges(ctx, name, []schema.Change{c}, opts...)
		if err != nil {
			return nil, err
		}
		if !plan.Reversible {
			s.Plan.Reversible = false
		}
		s.Plan.Changes = append(s.Plan.Changes, plan.Changes...)
	}
	return &s.Plan, nil
}

func (p *oplanApply) ApplyChanges(ctx context.Context, changes []schema.Change, opts ...migrate.PlanOption) error {
	return sqlx.ApplyChanges(ctx, changes, p, opts...)
}
