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
go run ./cmd tui
```
