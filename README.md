# Reeve CI plugin for Gitea

## Configuration

This plugin applies to and is expected to be used in common Git environments.

This plugin supports promoting actions to the WebUI plugin if present.
Actions can be logically grouped by using colons as separators, e.g. `parent-group:child-group:name`.
To hide actions from the WebUI, you can prefix them with a colon, e.g. `:name`.

### Gitea

An API token is required for this plugin.
Please select an user which has access to all relevant repositories.

You need to setup webhooks in your Gitea instance if you want to trigger pipelines on pushes.
The webhook URL should look like `http(s)://<YOUR_REEVE_SERVER>:<PORT>/api/v1/message?token=<YOUR-MESSAGE-SECRET>&target=gitea&type=webhook`.

For most configurations, you will want to give Reeve access to only a subset of the projects on your Git server.
To do this, you just need to assign the user for whom you are generating the API token to the appropriate repositories.
You can also enable access for a whole organization by adding the user to the organization instead of the individual repositories.
However, note that the user must have write access in order to be recognized as an assignee.

If you want to enable Reeve for the entire Git server instead, set the `UNRESTRICTED` setting to `true` and grant administrative access to the token user.

### Settings

Settings can be provided to the plugin through environment variables set to the reeve server.

Settings for this plugin should be prefixed by `REEVE_PLUGIN_GITEA_`.

Settings may also be shared between plugins by prefixing them with `REEVE_SHARED_` instead.

- `ENABLED` - `true` enables this plugin
- `URL` (required) - Gitea base URL (usually `https://git.your.domain`). This is used for accessing Gitea from the plugin, checking repository URL validity (unless `PUBLIC_URL` is configured) and for cloning repositories when running pipelines (unless `CLONE_URL` is configured).
- `CLONE_URL` - Optional Gitea base URL that workers should use for cloning repositories, if different than `URL`. This setting should be used if your workers cannot access Gitea over `URL`.
- `PUBLIC_URL` - Optional Gitea base URL for checking repository URL validity, if different than `URL`. This is the URL that Gitea is publicly available at (this is configured in Gitea as `ROOT_URL`). This setting should be used if the plugin accesses Gitea from another URL than your users do. Note that Reeve won't be able to run any Gitea pipelines if this does not match what the Gitea ReST API returns.
- `TOKEN` (required) - Gitea API Token
- `UNRESTRICTED` - Do not restrict search by user
- `TASK_DOMAINS` - Space separated list of task domains. Each entry should have the form `domain` or `domain:prefix`, where domain is the name of the task domain and prefix is an optional prefix for the domain's tasks. If an entry contains multiple colons, the first colon is used as the separator.
- `TRUSTED_DOMAINS` - Space separated list of task domains to trust. A task is considered to be trusted if it has a task domain specified and if the task domain matches one of the options provided by this setting.
- `TRUSTED_TASKS` - Space separated list of tasks to trust. A task is considered to be trusted if it matches one of the options provided by this setting.
- `SETUP_GIT_TASK` (required) - Task to be used for setting up pipelines
- `SECRET_KEY` (required) - Passphrase for encrypting secrets
- `DISCOVERY_SCHEDULE` (defaults to `"0 12 * * *"`) - Cron expression which specifies how often the Git server should be fully scanned. The server is also scanned when the plugin starts, and single repositories are updated when a corresponding webhook is received. Scheduled server scanning can be disabled by setting the option to `never`.

### Messages

This plugin supports two types of messages:

#### Gitea webhooks

Gitea webhooks allow Reeve to run pipelines whenever a specific action is executed in your Git repositories.

You can skip pipeline execution by adding `[skip ci]` or `[ci skip]` anywhere in your commit message.

**Query parameters:**

- `token` - Reeve message secret
- `target` - Must be `gitea`
- `type` - Must be `webhook`

**Content:**

See Gitea Webhook API

#### Actions

Actions can be used to execute pipelines via HTTP requests to the message endpoint.
Unless the `UNRESTRICTED` setting is enabled, the execution of actions is restricted to projects to which the token user is assigned.

**Query parameters:**

- `token` - Reeve message secret
- `target` - Must be `gitea`
- `type` - Must be `action`
- `action` - Action to be passed to pipeline facts
- `search` - Search term for limiting repository discovery

Actions can also be triggered via the CLI API:

```sh
reeve-cli --url <server-url> gitea action <action> [<search> ...]
```

### Facts

The following facts are provided:

- `trigger` - [`push`, `commit`] or [`push`, `tag`] or [`action`]
- `action` - Specified action - Only available for `action` triggers
- `ref` - Git ref - Ref of the head commit or tag, e.g. `refs/heads/main` or `refs/tags/v1.0.0`
- `branch` - Git branch - Not available for `tag` triggers
- `file` - Affected file(s) - Only available for `commit` triggers
- `tag` - Git tag - Only available for `tag` triggers
- `repository` - Full name of the repository, e.g. `ReeveCI/Reeve`

> Using the `file` fact when force-pushing changes may result in unexpected behavior, as monitoring file changes is limited to commits that are not already known to Gitea.
> If, for example, a branch was reset to a previous commit and then force-pushed, no new commits would be pushed, so no files would be marked as changed, even if the working directory has changed.

### Default conditions

If not specified otherwise, pipelines are limited to commits on the repository's default branch.
This can be changed by adding conditions for `trigger` and `branch` in your pipelines `when` section.

Since it is usually undesirable to execute a pipeline without restriction for all possible actions if the `action` trigger is set, this is prevented by default.
Therefore actions must always be specified explicitely by also adding conditions for `action`.

### Pipeline definition

Pipelines and environment variables are defined in the `/.reeve.yaml` file in a repository's root directory.

The file can contain multiple YAML documents, e.g. (using a docker runner):

```yaml
---
type: variable
name: MY_VAR
value: some-value

---
type: pipeline
name: hello-world

steps: []
```

#### Variables

```yaml
---
type: variable
name: MY_ENV
value: some-value
```

#### Secrets

```yaml
---
type: secret
name: MY_ENV
value: some-encrypted-value
```

Values can be encrypted using the `encrypt` CLI method:

```sh
reeve-cli --url <server-url> gitea encrypt "<secret value>"
```

Encryption takes place on the server, so make sure to use a secure connection between reeve-cli and the server. That is, use TLS with a valid certificate and do not set the `--insecure` flag.

#### Cron schedules

```yaml
---
type: trigger
cron: * * * * *
action: some-action
```

The specified action is triggered based on the schedule.

Cron syntax:

```
*     *     *     *     *

^     ^     ^     ^     ^
|     |     |     |     |
|     |     |     |     +----- day of week (0-6) (Sunday=0)
|     |     |     +------- month (1-12)
|     |     +--------- day of month (1-31)
|     +----------- hour (0-23)
+------------- min (0-59)
```

Examples:

- `* * * * *` - Run on every minute
- `0 0 * * 1` - Run at midnight on every Monday
- `* 10,15,19 * * *` - run at 10:00, 15:00 and 19:00
- `1-15 * * * *` - run at 1, 2, 3...15 minute of each hour
- `*/2 * * * *` - run every two minutes
- `1-59/2 * * * *` - run every two minutes, but on odd minutes

Details: https://github.com/mileusna/crontab

#### Pipelines

```yaml
---
type: pipeline
name: hello-world
description: |
  # Markdown description for your pipeline

when:
  some-fact:
    include: [value]
    exclude: [value]
    include env: [MY_ENV]
    exclude env: [MY_ENV]
    match: [^regexp$]
    mismatch: [^regexp$]
  env MY_ENV:

steps:
  - name: greet
    stage: greeting
    task: hello-world
    command: ["sh", "-c", "echo hello-world"]
    input: |
      data to be sent
      to stdin
    directory: /host/directory/to/be/mounted
    user: "user-or-uid"
    params:
      PARAM1: some-value
      PARAM2: { env: MY_ENV, replace: [/regexp/replacement/] }
      PARAM3: { var: MY_VAR, replace: [] }

    ignoreFailure: true

    when:
      fact:
        include: [value]
        exclude: [value]
        include env: [MY_ENV]
        exclude env: [MY_ENV]
        include var: [MY_ENV]
        exclude var: [MY_ENV]
        match: [^regexp$]
        mismatch: [^regexp$]
      env MY_ENV:
      var MY_VAR:
```
