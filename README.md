# SYSTEM - Solo Leveling Style Habit Tracker over SSH

A **Solo Leveling**–style daily habit tracker over SSH. Connect with SSH, then log in with your username and password in the app. Each account has its own quest log, level, and EXP.
```bash
ssh -p 50526 system.hostagedown.com
```

## Features

- **Username & password login** — After SSH connect, enter your credentials in the TUI (no SSH username = account)
- **Register** — New users press `[r]` on the login screen to create an account
- **Daily quests** — Add habits as “daily quests”; complete them each day for EXP
- **Level & EXP** — +10 EXP per quest; level up every 100 EXP
- **Custom Reset Time** — Press `[s]` to set when your day resets (default 4 AM)
- **Solo Leveling UI** — System window, cyan borders, EXP rewards, level bar, time progress bar

## Run

**Local (Go):**
```bash
go run ./cmd/server
```
The server auto-generates an SSH host key on first run if missing.

**Docker:**
```bash
docker compose up -d
```
User data is stored in the `system_data` volume. Connect with `ssh -p 23234 user@localhost`.

## Connect

**Local:**
```bash
ssh -p 23234 user@localhost
```

**Production:**
```bash
ssh -p 50526 system.hostagedown.com
```

(Configure the host so SSH uses port 23234 and the system user if needed.)

After connecting, the app shows **SYSTEM — LOGIN**. Enter your username, press Tab, enter your password, then Enter to log in. New users: press **r** to open the register screen, then enter username and password and Enter to create an account.

## Login / Register

| Key        | Action                |
|-----------|------------------------|
| **Tab**   | Switch between username and password |
| **Enter** | Submit (login or create account)     |
| **r**     | (on login) Switch to register       |
| **Esc**   | (on register) Back to login         |
| **q**     | Quit                                 |

Password is masked (•••). Usernames are stored lowercase.

## Main app

| Key        | Action                |
|-----------|------------------------|
| `a`       | Add new daily quest    |
| `d` / `x` | Delete selected quest  |
| `Space`   | Toggle complete today  |
| `↑` / `k` | Move up                |
| `↓` / `j` | Move down              |
| `q`       | Quit                   |

When adding a quest: type the name, **Enter** to save, **Esc** to cancel.

## Data

- Stored under `data/<username>.json` (passwords are bcrypt hashes)
- Daily completions are per calendar day; next day all quests reset (new day = new EXP to earn)
- In Docker, mount a volume at `/app/data` to persist user data (e.g. `docker compose` does this with `system_data`)
