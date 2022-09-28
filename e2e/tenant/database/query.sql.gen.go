// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.14.0
// source: query.sql

package database

import (
	"context"
)

const clearEmployees = `-- name: ClearEmployees :exec
delete from employees
`

func (q *Queries) ClearEmployees(ctx context.Context) error {
	_, err := q.db.Exec(ctx, clearEmployees)
	return err
}

const clearTenants = `-- name: ClearTenants :exec
delete from tenants
`

func (q *Queries) ClearTenants(ctx context.Context) error {
	_, err := q.db.Exec(ctx, clearTenants)
	return err
}

const createEmployee = `-- name: CreateEmployee :one
insert into employees (
    username, name, tenant_id
) values (
    $1, $2, $3
) returning id, username, name, tenant_id
`

type CreateEmployeeParams struct {
	Username string `db:"username"`
	Name     string `db:"name"`
	TenantID int32  `db:"tenant_id"`
}

func (q *Queries) CreateEmployee(ctx context.Context, arg CreateEmployeeParams) (Employee, error) {
	row := q.db.QueryRow(ctx, createEmployee, arg.Username, arg.Name, arg.TenantID)
	var i Employee
	err := row.Scan(
		&i.ID,
		&i.Username,
		&i.Name,
		&i.TenantID,
	)
	return i, err
}

const createTenant = `-- name: CreateTenant :one
insert into tenants (
    name
) values (
    $1
)
returning id, name
`

func (q *Queries) CreateTenant(ctx context.Context, name string) (Tenant, error) {
	row := q.db.QueryRow(ctx, createTenant, name)
	var i Tenant
	err := row.Scan(&i.ID, &i.Name)
	return i, err
}

const getEmployees = `-- name: GetEmployees :one
select id, username, name, tenant_id from employees where tenant_id = $1 and id = $2 limit 1
`

type GetEmployeesParams struct {
	TenantID int32 `db:"tenant_id"`
	ID       int32 `db:"id"`
}

func (q *Queries) GetEmployees(ctx context.Context, arg GetEmployeesParams) (Employee, error) {
	row := q.db.QueryRow(ctx, getEmployees, arg.TenantID, arg.ID)
	var i Employee
	err := row.Scan(
		&i.ID,
		&i.Username,
		&i.Name,
		&i.TenantID,
	)
	return i, err
}

const getTenant = `-- name: GetTenant :one
select id, name from tenants where id = $1 limit 1
`

func (q *Queries) GetTenant(ctx context.Context, id int32) (Tenant, error) {
	row := q.db.QueryRow(ctx, getTenant, id)
	var i Tenant
	err := row.Scan(&i.ID, &i.Name)
	return i, err
}

const listEmployees = `-- name: ListEmployees :many
select id, username, name, tenant_id from employees where tenant_id = $1
`

func (q *Queries) ListEmployees(ctx context.Context, tenantID int32) ([]Employee, error) {
	rows, err := q.db.Query(ctx, listEmployees, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Employee
	for rows.Next() {
		var i Employee
		if err := rows.Scan(
			&i.ID,
			&i.Username,
			&i.Name,
			&i.TenantID,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const listTenants = `-- name: ListTenants :many
select id, name from tenants
`

func (q *Queries) ListTenants(ctx context.Context) ([]Tenant, error) {
	rows, err := q.db.Query(ctx, listTenants)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Tenant
	for rows.Next() {
		var i Tenant
		if err := rows.Scan(&i.ID, &i.Name); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}