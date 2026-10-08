<!-- Generated from the command tree by `make docs`. Do not edit. -->

# sonde export html

Print .hurl files as syntax-highlighted HTML.

```
sonde export html [FILE...] [flags]
```

Html prints each .hurl file as a highlighted <pre> block, or with
--standalone as a complete HTML document with its stylesheet. With no
FILE it reads standard input.

## Flags

```
  -o, --output string   write to FILE instead of stdout
      --standalone      write a complete HTML document with its stylesheet
```

## Global flags

```
      --color      colorize output
      --no-color   do not colorize output
```

Parent command: [sonde export](sonde_export.md)
