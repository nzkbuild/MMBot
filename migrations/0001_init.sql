-- Core schema for production Postgres deployment.

create table if not exists broker_accounts (
    id text primary key,
    broker_name text not null,
    mode text not null default 'paper',
    created_at timestamptz not null default now()
);

create table if not exists ea_devices (
    id text primary key,
    account_id text not null references broker_accounts(id),
    token_hash text not null,
    token_expires_at timestamptz not null,
    created_at timestamptz not null default now(),
    last_seen_at timestamptz
);

create table if not exists commands (
    id text primary key,
    account_id text not null references broker_accounts(id),
    type text not null,
    symbol text,
    side text,
    volume numeric(18,8),
    sl numeric(18,8),
    tp numeric(18,8),
    reason text,
    status text not null,
    expires_at timestamptz not null,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);
create index if not exists idx_commands_account_status on commands(account_id, status);

create table if not exists events (
    id text primary key,
    account_id text,
    event_type text not null,
    payload jsonb not null,
    created_at timestamptz not null default now()
);
create index if not exists idx_events_created_at on events(created_at desc);
create index if not exists idx_events_event_type on events(event_type);

create table if not exists oauth_provider_connections (
    provider text primary key,
    access_token_enc text not null,
    refresh_token_enc text,
    scopes text[] not null default '{}',
    expires_at timestamptz not null,
    connected_at timestamptz not null default now()
);

create table if not exists daily_risk_state (
    account_id text primary key references broker_accounts(id),
    daily_loss_pct numeric(10,4) not null default 0,
    open_positions int not null default 0,
    paused boolean not null default false,
    updated_at timestamptz not null default now()
);

