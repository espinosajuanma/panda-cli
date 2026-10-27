# Panda Dashboard

Panda Dashboard is a CLI tool written in go that connects with the Solutions
app REST API to sync the current tasks from the task dashboard and allow the user
to update them using the file system.

## Requirements

- Go 1.26+
- No database
- File system

## Capabilities

- Sync Tasks > Bring all the tasks from solutions to files in local
- Help Command like man pages for every command
- Every HTML field must be fetch and transformed to markdown locally
- Every HTML field locally in markdown must be transformed back to HTML to push
  to the solutions app

## Commands

Read assets/solutions-api.json for the whole `paths` documentation

### `login`

- Read User Input Email and Password
- HTTP POST `/auth/login` `{email,password}`
- Saves the `token` in a local configuration file
- Use that token as header for the subsequent HTTP requests

### `config set project {projectName}`

- Validates project exist -> `/projects?name={projectName}`
- Updates the project id and name in local configuration

### `config get project`

- Prints the current project

### `pull`

- If project is not set throws error
- Fetch all the changes from the solutions app
- The user can edit the files locally using the commands
- Changes remain local until a `push` is proceeded

### `push`

- If project is not set throws error
- Takes all the local changes and impact them in solutions app using the API

### `diff`

- If project is not set throws error
- It display all the changes locally that are going to be impacted in the
  solutions app

### `create`

- Creates a task locally

### `discard {taskNumber}`

- Mark a task to discard locally
- Add optional markdown note (to be transformed to html)

### `edit {taskNumber}`

- Edit a task locally

### `view {taskNumber}`

- Display the ticket information in highlighted markdown

### `add-note {taskNumber}`

- Add a note locally

## Tasks

I want to be able to edit these:

- Title (Text)
- Type (Enum)
- Priority (Enum)
- Complexity (Enum)
- Details (HMTL)
- Labels (Relationship -> `dev.labels`)

And track these:

- Status
- Assignees
- Reviewers
- Notes

## File System

- Must be intuitive and easy to see the tasks metadata

- `{projectName}`
  - `{taskNumber`
    - description.md
    - metadata.json
    - notes
      - {timeStamp} - {authorName}.md

## CLI

- Similar to the `gh` tool that is interactive
- For long body like the description allows you to type `e` to open the `$EDITOR`
- Colors
- Autocomplete for labels
