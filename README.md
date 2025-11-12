# atuin-fzf

Combine [atuin](https://github.com/atuinsh/atuin) and [fzf](https://github.com/junegunn/fzf) for a familiar yet powerful search of command history.

## Preview

<img width="958" height="577" alt="image" src="https://github.com/user-attachments/assets/18ba7056-855a-4687-a785-f508b93d9d18" />

## Why?

atuin is great for extended command history, it stores additional metadata (directory, duration), uses a database, and supports synchronization across machines.
However, I found the interface lacking compared to fzf, so I thought why not use atuin as the backend, and fzf as the interface.

## Install

 * Install atuin as per the [quickstart](https://github.com/atuinsh/atuin?tab=readme-ov-file#quickstart) or [install](https://docs.atuin.sh/guide/installation/) directions.
 * Update the atuin integration so it's not triggered for interactive use: `eval "$(atuin init zsh --disable-up-arrow --disable-ctrl-r)"`
 * Download `atuin-fzf`
 * Enable `atuin-fzf` to be used for Ctrl-R:

```
eval "$(/Users/prashant/go/src/prashantv/atuin-fzf/atuin-fzf --zsh)"
```

Note: Only bash is supported.

## Features

* Shows the exit status, and whether commands were run in the current directory as part of the primary fzf view.
* Uses fzf previews to show more details about the comamnd (where it was run, duration, other similar commands)
* Allows changing directory into the directory where a previous command was run (Ctrl-O).
