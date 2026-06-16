# epsilon

![](./assets/header.jpg)

## Quickstart

LiteLLM has been configured as the only provider so far.

```sh
export EPSILON_PROVIDER=litellm
export LITELLM_BASE_URL=https://your_litellm_url
export LITELLM_MODEL=gpt-5.4
export LITELLM_API_KEY=your_api_key
```

Then run:

```sh
go run ./cmd/epsilon-cli/ tui
```

## TUI slash commands

Slash commands run locally in the TUI and are not sent to the model. Use `//` to send
a literal message that starts with `/`.

Type `/` in the composer to open a fuzzy command selector. Use up/down to move through
matches and tab to complete the highlighted command.

- `/help` - show available slash commands
- `/clear` - clear the visible transcript
- `/events [on|off|toggle]` - show or hide event entries
- `/density [comfortable|compact|toggle]` - switch transcript density
- `/model [name]` - pick or change the model
- `/effort [minimal|low|medium|high|off]` - show or change model effort
- `/status` - show session and TUI state
- `/quit` - quit epsilon
