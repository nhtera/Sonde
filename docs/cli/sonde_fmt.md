<!-- Generated from the command tree by `make docs`. Do not edit. -->

# sonde fmt

Format request files canonically.

```
sonde fmt [FILE...] [flags]
```

Fmt prints files in canonical layout: whitespace normalized, sections in
canonical order ([Options] first), and a unit (ms) added to unitless
durations; bodies are untouched. With no FILE it reads standard input.
--write rewrites files in place. --check lists files that are not
formatted and exits with 1 (2 if a file cannot be read or parsed).

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
