create table if not exists tenants (
    id serial not null,
    name text not null,
    primary key (id)
);

create table if not exists employees (
    id serial not null,
    name text not null,
    tenant_id integer not null,
    foreign key (tenant_id)
        references tenants (id)
        on delete no action
        on update no action
);
