# JQL Support

`jira issue list` starts with a bounded default query:

```jql
updated >= -365d ORDER BY updated DESC
```

To restrict the default to issues assigned to you, pass `--assignee me`.

Use raw JQL when you already know it:

```sh
jira issue list --jql 'project = PROJ AND status = "In Progress"'
```

Use the builder when you do not:

```sh
jira jql build --project PROJ --assignee me --status "In Progress"
jira issue list --project PROJ --assignee me --status "In Progress"
```

Builder flags map to documented Jira JQL concepts:

- Fields: `--project`, `--epic`, `--assignee`, `--reporter`, `--status`,
  `--priority`, `--label`, `--type`
- Sort: `--order-by <field>`, `--desc=false` for ascending order
- Operators applied: `=`, `in (...)` (for repeated flag values), `is EMPTY`
- Keywords/functions: `AND`, `ORDER BY`, `currentUser()` (via `--assignee me`
  or `--reporter me`)

Atlassian references:

- [JQL fields](https://support.atlassian.com/jira-service-management-cloud/docs/jql-fields/)
- [JQL operators](https://support.atlassian.com/jira-service-management-cloud/docs/jql-operators/)
- [JQL keywords](https://support.atlassian.com/jira-service-management-cloud/docs/jql-keywords/)
- [JQL developer status](https://support.atlassian.com/jira-service-management-cloud/docs/jql-developer-status/)
- [Advanced Roadmaps custom fields](https://support.atlassian.com/jira-service-management-cloud/docs/search-for-advanced-roadmaps-custom-fields-in-jql/)

Sort defaults to descending. For example, `jira jql build --project PROJ`
emits `project = PROJ ORDER BY updated DESC`; pass `--desc=false` for
ascending order.
