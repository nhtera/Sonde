<!-- Generated from the command tree by `make docs`. Do not edit. -->

# sonde export curl

Print each entry's equivalent curl command line, without sending it.

```
sonde export curl FILE... [flags]
```

## Flags

```
      --config string                uses this sonde.yaml for every input file, skipping discovery
      --entry int                    exports only the given entry number (1-based); 0 exports every entry
      --env string                   selects a sonde.yaml environment by name
      --file-root string             sets the root directory used to resolve file paths
      --secret stringArray           defines a variable whose value is treated as a secret
      --secrets-file stringArray     defines secrets from a file
      --variable stringArray         defines a variable
      --variables-file stringArray   defines variables from a properties file
```

## Global flags

```
      --color      colorize output
      --no-color   do not colorize output
```

Parent command: [sonde export](sonde_export.md)
