-- name: CreateTenant :one
insert into tenants (
    name
) values (
    $1
)
returning *;

-- name: CreateTenants :copyfrom
insert into tenants (
    name
)values (
    $1
);

-- name: ListTenants :many
select * from tenants;

-- name: GetTenant :one
select * from tenants where id = $1 limit 1;

-- name: ClearTenants :exec
delete from tenants;

-- name: CreateEmployee :one
insert into employees (
    username, name, tenant_id
) values (
    $1, $2, $3
) returning *;

-- name: CreateEmployees :copyfrom
insert into employees (
    username, name, tenant_id
) values (
    $1, $2, $3
);

-- name: ListEmployees :many
select * from employees where tenant_id = $1;

-- name: GetEmployees :one
select * from employees where tenant_id = $1 and id = $2 limit 1;

-- name: ClearEmployees :exec
delete from employees;
