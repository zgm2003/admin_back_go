# Local Database Seeds

## Admin permissions

`admin_permissions.sql` is the canonical local seed for the current Admin menu
and permission tree. It contains 125 active Admin rows with stable IDs so all
parent-child relationships remain deterministic.

Apply it only to an empty `permissions` table. The seed intentionally fails
before inserting rows when the table already contains data. It is an explicit
initialization operation and is never run by API or Worker startup.

From PowerShell, with the local MySQL state container running:

```powershell
Get-Content database/seeds/admin_permissions.sql -Raw |
  docker exec -i admin-state-mysql-1 sh -lc 'mysql --default-character-set=utf8mb4 -uroot -p"$(cat /run/secrets/mysql_root_password)" admin'
```

This seed does not create or modify users or roles, and it does not assign
permissions to any role. First-administrator creation and authorization belong
to a future project-initialization flow.
