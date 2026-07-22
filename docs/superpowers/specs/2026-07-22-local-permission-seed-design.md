# Local Database Initialization Data Design

**Date:** 2026-07-22

## Goal

Prepare the current local database's intentional initial data state:

1. turn the complete active Admin permission tree into a tracked,
   deterministic seed for an empty local `permissions` table;
2. remove payment data other than the retained payment configuration.

## Scope

The seed contains the current 125 active `admin` permission rows and preserves:

- stable permission IDs and parent-child relationships;
- names, paths, icons, components, and platform;
- directory, page, and button types;
- sort values, permission codes, i18n keys, menu visibility, and status.

Only active Admin rows are included. Retired, soft-deleted, App, and Canvas
permission data is excluded.

## Explicit Non-Goals

This stage does not:

- create or modify users;
- create or modify roles;
- write `role_permissions` grants;
- declare, generate, or authorize a super-admin account;
- run automatically during API or Worker startup;
- overwrite or reconcile an already customized permission table.

The future full project-initialization flow will separately collect the first
administrator account, create its role, and grant the seeded permissions.

## Local Payment Data Reset

The reset preserves `payment_configs` byte-for-byte and clears every row from:

- `payment_callback_events`;
- `payment_recharges`;
- `payment_orders`;
- `payment_recharge_packages`;
- `wallet_transactions`;
- `user_wallets`.

Child records are cleared before parent records. The reset also returns the
cleared tables' auto-increment counters to their empty-table state. It does not
change menu permissions or any user, role, or role grant.

The reset is a separate explicit local initialization operation. It does not
run during API or Worker startup and is not embedded in the permission seed.

## Seed Placement And Execution

The canonical data belongs under `database/seeds/`, not in the Atlas schema
migration directory. Schema migrations remain structural and continue to obey
the rule that application startup never applies migrations.

A local initialization command will apply the seed in one transaction. It must
first prove that `permissions` is empty. A non-empty table is a hard stop so the
command cannot overwrite user-managed menu data. The current populated local
database is the source used to generate and verify the tracked seed; it does
not need the seed replayed onto it.

## Validation

Automated validation must prove that:

- exactly 125 rows are present in the seed;
- all rows use platform `admin`, active status, and active deletion state;
- IDs and non-empty permission codes are unique;
- every non-root `parent_id` resolves to another seeded row;
- no retired product or client-version permission is present;
- the deterministic seeded row projection matches the current local database;
- the initialization path contains no write to `users`, `roles`, or
  `role_permissions`.

Payment reset verification must prove that all six cleared tables contain zero
rows and that the retained `payment_configs` count and deterministic row hash
are unchanged.

An empty disposable database verification will apply the canonical schema and
the permission seed, then check the same invariants without creating accounts
or grants.
