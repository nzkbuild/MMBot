create table if not exists app_state (
    key text primary key,
    value_json jsonb not null,
    updated_at timestamptz not null default now()
);

create table if not exists position_snapshots (
    account_id text primary key references broker_accounts(id),
    snapshot jsonb not null,
    updated_at timestamptz not null default now()
);

