# Pull Request Size Labeler

GitHub Action to label Pull Requests according to the number of modified lines.

## Features

- `diffstat -m` based calculation that only counts modified lines, instead of double counting additions and deletions.
- Configurable thresholds for each label (XS, S, M, L).
- Configurable labels for each threshold.
- Exclude paths from calculation.
- Optionally fail and comment when a PR exceeds the largest threshold.

## Usage

Create a file named `pr-size-labeler.yml` inside the `.github/workflows` directory, then modify and use the following config:

```yml
name: pr-size-labeler

on: [pull_request]

jobs:
  labeler:
    runs-on: ubuntu-latest
    name: Label the PR size
    steps:
      - uses: RoryQ/pr-size-labeler@master
        with:
          github_token: ${{ secrets.GITHUB_TOKEN }}
          # Thresholds or labels can be overridden. These values are the defaults so can be omitted
          thresholds: |
            xs:
              less_than: 10
              label: size/xs
            s:
              less_than: 100
              label: size/s
            m:
              less_than: 500
              label: size/m
            l:
              less_than: 1000
              label: size/l
            # Set to true if an XL should fail the PR
            fail_if_xl: false
            message_if_xl: This PR is too big. Please, split it.
          # List the filepaths to exclude like below. Supports globs. Default is empty.
          exclude_paths: |
            - go.mod
            - go.sum
```