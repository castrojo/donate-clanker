# Reviewer Client Banner Design

The managed reviewer client needs a compact terminal marker that makes its
human decision gate obvious without implying that a pull request was approved,
merged, or otherwise completed.

## Presentation

At the start of the managed reviewer client, print this exact ASCII-only
banner:

```text
+------------------------+
| BLUEFIN REVIEW         |
| HUMAN DECISION REQUIRED|
+------------------------+
```

The banner has no color, animation, terminal detection, timestamp, or
completion language. It is an orientation marker, not lifecycle evidence.

## Implementation

`image/bin/bluefin-review` replaces its existing one-line identity output with
the three-line banner and continues to `exec goose review "$@"` unchanged.
No launcher, Hive, queue, persistence, credential, argument, or exit-status
behavior changes.

## Validation

Add a focused test that asserts the wrapper emits the exact banner and
forwards its arguments to `goose review`.
