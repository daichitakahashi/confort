create table tenants (
    id serial not null,
    name text not null,
    primary key (id)
);

create table employees (
    id serial not null,
    name text not null,
    tenant_id integer not null,
    foreign key (tenant_id)
        references tenants (id)
        on delete no action
        on update no action
);
