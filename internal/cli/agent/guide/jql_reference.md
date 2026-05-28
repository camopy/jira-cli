## jql_reference
When to use this: JQL is the query language behind → `search_jql` and → `list_issues`. Use this section as a lookup for the operators, keywords, functions, and recipes that compose into a query string. For the `jira jql build` flag-driven constructor and `jira search saved`, see → `search_jql`.

Authoritative Atlassian docs (cite these — link rendering is honest):

- [JQL fields](https://support.atlassian.com/jira-service-management-cloud/docs/jql-fields/)
- [JQL operators](https://support.atlassian.com/jira-service-management-cloud/docs/jql-operators/)
- [JQL keywords](https://support.atlassian.com/jira-service-management-cloud/docs/jql-keywords/)
- [JQL functions](https://support.atlassian.com/jira-service-management-cloud/docs/jql-functions/)
- [JQL developer status](https://support.atlassian.com/jira-service-management-cloud/docs/jql-developer-status/)
- [JQL advanced-roadmap fields](https://support.atlassian.com/jira-service-management-cloud/docs/search-for-advanced-roadmaps-custom-fields-in-jql/)

### Common operators

| Operator                | Meaning                                  |
|-------------------------|------------------------------------------|
| `=`  /  `!=`            | exact match                              |
| `in (a, b, c)`          | match any of                             |
| `not in (a, b, c)`      | match none of                            |
| `~` / `!~`              | text match (string fields)               |
| `>`  `>=`  `<`  `<=`    | numeric / date comparison                |
| `is empty` / `is not empty` | null check (some fields use `EMPTY`) |
| `was`                   | historical value (combined with `during(...)`) |
| `changed`               | value transitioned (combined with `from`/`to`/`by`/`during`) |

### Common keywords

| Keyword | Meaning                              |
|---------|--------------------------------------|
| `AND`   | all conditions must match            |
| `OR`    | any condition may match              |
| `NOT`   | invert the condition                 |
| `ORDER BY <field> ASC|DESC` | sort the result set      |

### Common functions

```text
currentUser()              # the calling user's accountId
now()                      # current timestamp
startOfDay() / endOfDay()  # boundary helpers (also Week/Month/Year)
membersOf("group-name")    # members of a Jira group
componentsLeadByUser()     # components led by current user
projectsLeadByUser()
linkedIssues(KEY [, "blocks"])   # find issues linked to KEY by a link type
issuesWithText("phrase")
```

### High-yield recipes

```sh
# Build a key filter from mixed projects without hand-writing IN clauses
jira jql build --key <PROJECT_KEY>-1:10,<OTHER_PROJECT_KEY>-1:12 --output=json

# Everything assigned to me, not done
jira search jql 'assignee = currentUser() AND statusCategory != Done ORDER BY updated DESC' --output=json

# In-flight issues in a specific project
jira search jql 'project = <PROJECT_KEY> AND status = "In Progress"' --output=json

# Bugs reported in the last sprint
jira search jql 'project = <PROJECT_KEY> AND issuetype = Bug AND created > startOfMonth()' --output=json

# Issues in any of my epics
jira search jql 'project = <PROJECT_KEY> AND parent in (linkedIssues(currentUser()))' --output=json

# Recently updated, with a specific label
jira search jql 'project = <PROJECT_KEY> AND labels = "regression" AND updated > -7d' --output=json

# Issues blocked by a specific issue
jira search jql 'issue in linkedIssues("<ISSUE_KEY>", "is blocked by")' --output=json

# Subtasks of a parent
jira search jql 'parent = <PARENT_ISSUE_KEY>' --output=json

# Status-history check (was = 'In Progress' some time recently)
jira search jql 'status was "In Progress" during ("2026-04-01", "2026-05-01")' --output=json
```
