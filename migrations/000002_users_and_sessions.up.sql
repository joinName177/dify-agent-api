CREATE TABLE app.users (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id uuid NOT NULL DEFAULT gen_random_uuid(),
    email text NOT NULL,
    display_name text NOT NULL,
    password_hash text NOT NULL,
    role text NOT NULL DEFAULT 'user',
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT users_public_id_unique UNIQUE (public_id),
    CONSTRAINT users_email_normalized CHECK (email = lower(email)),
    CONSTRAINT users_email_length CHECK (char_length(email) BETWEEN 3 AND 254),
    CONSTRAINT users_display_name_length CHECK (char_length(display_name) BETWEEN 1 AND 80),
    CONSTRAINT users_role_valid CHECK (role IN ('user', 'super_admin')),
    CONSTRAINT users_status_valid CHECK (status IN ('active', 'disabled'))
);

CREATE UNIQUE INDEX users_email_unique ON app.users (email);

CREATE TABLE app.auth_sessions (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id bigint NOT NULL REFERENCES app.users(id) ON DELETE CASCADE,
    access_token_hash bytea NOT NULL,
    refresh_token_hash bytea NOT NULL,
    access_expires_at timestamptz NOT NULL,
    refresh_expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT auth_sessions_access_token_hash_unique UNIQUE (access_token_hash),
    CONSTRAINT auth_sessions_refresh_token_hash_unique UNIQUE (refresh_token_hash),
    CONSTRAINT auth_sessions_expiry_order CHECK (access_expires_at <= refresh_expires_at)
);

CREATE INDEX auth_sessions_user_id_index ON app.auth_sessions (user_id);
CREATE INDEX auth_sessions_active_access_index
    ON app.auth_sessions (access_token_hash, access_expires_at)
    WHERE revoked_at IS NULL;
CREATE INDEX auth_sessions_active_refresh_index
    ON app.auth_sessions (refresh_token_hash, refresh_expires_at)
    WHERE revoked_at IS NULL;

