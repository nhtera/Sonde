<!-- Generated from the command tree by `make docs`. Do not edit. -->

# sonde fmt

Format request files canonically.

```
sonde fmt FILE... [flags]
```

Fmt prints files in canonical layout (whitespace only; nothing is reordered
and bodies are untouched). --write rewrites files in place, --check lists
files that are not formatted and exits with 1.

## Flags

```
      --check   list files that are not formatted; exit 1 if any
  -w, --write   write the result to the files instead of stdout
```

## Global flags

```
      --color      colorize output
      --no-color   do not colorize output
```

Parent command: [sonde](sonde.md)
