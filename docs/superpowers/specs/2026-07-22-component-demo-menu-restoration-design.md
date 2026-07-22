# Component Demo Menu Restoration Design

**Date:** 2026-07-22

## Goal

Restore the Admin component demonstration menu that was physically removed
while retaining the existing browser-only implementation.

## Chosen Approach

Restore the historical stable IDs and database-driven routes. This preserves
the original parent-child relationship and matches the frontend route registry
and locale keys that are still present. Allocating new IDs would discard useful
history, while a frontend-only static menu would bypass the permission model.

## Permission Tree

The restored active Admin rows are:

| ID | Parent | Type | Name | Path / component | i18n key | Sort |
| --- | --- | --- | --- | --- | --- | --- |
| 4 | 0 | directory | 组件演示 | empty | `menu.component` | 4 |
| 40 | 4 | page | 上传 | `/component/upload` | `menu.component_upload` | 1 |
| 41 | 4 | page | 表单 | `/component/form` | `menu.component_form` | 2 |
| 42 | 4 | page | 展示 | `/component/display` | `menu.component_display` | 3 |
| 43 | 4 | page | 特效 | `/component/effect` | `menu.component_effect` | 4 |
| 80 | 4 | page | 下载管理器 | `/component/download` | `menu.component_download` | 5 |

All six rows use `platform = 'admin'`, active status, active deletion state,
and visible menu state. Page components omit the leading slash, matching the
generated frontend route keys. The directory uses the existing `Menu` icon.

## Local Authorization

The only retained local user uses role ID 2 (`超管`). Add active
`role_permissions` rows for the five page IDs. Directory permissions are not
assigned directly in the existing model; the principal menu builder includes
their active parent directory. Do not modify role ID 1 and do not add grants to
the initialization seed.

## Tracked Initialization Data

Update `database/seeds/admin_permissions.sql` from 125 to 131 rows and keep the
six restored rows in strict ID order. Update the architecture test to assert
the new row count and exact component route tree. Account, role, and grant
writes remain forbidden in the seed.

## Frontend Scope

No frontend source change is required. All five Vue pages, generated dynamic
route entries, and Chinese and English menu locale keys already exist.

## Verification

Verification must prove:

- the architecture test fails before the seed update and passes afterward;
- a disposable schema loads exactly 131 seed rows and rejects a second load;
- the local database contains the six restored rows and five role-2 grants;
- the authenticated menu response contains all five component routes;
- backend architecture and full Go tests pass;
- both repositories remain on clean `master`, with backend pushed to
  `origin/master`.
