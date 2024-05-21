# paramsmap

A simple tool to help discovering hidden parameters. 

## Installation

```bash
go install -v github.com/pyneda/paramsmap@latest
```

or

```
git clone https://github.com/pyneda/paramsmap
cd paramsmap
go build
```

## Usage 

Scan a URL:

```bash
paramsmap -url "https://example.com" -wordlist params.txt -chunk-size 500
```

See all options:

```bash
paramsmap -h
```

## Credits

The discovery approach is based on the methodology used in [Arjun](https://github.com/s0md3v/Arjun), as described [here](https://github.com/s0md3v/Arjun/wiki/How-Arjun-works%3F).


