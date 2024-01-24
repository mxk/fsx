# File System Toolbox

WORK IN PROGRESS

fsx is a command-line tool designed primarily to help with file deduplication. It also includes subcommands for managing Volume Shadow Copies on Windows.

## File Deduplication Overview

fsx provides commands for identifying and removing duplicate files and directories. [BLAKE3] hash function is used to compare file contents. A description of the file system state is stored in a compressed text-based index file (format described below). The general workflow is:

1. Build or update an index of the file system.
2. Identify possible duplicate directories.
3. Categorize directories as either keep or delete.
4. Execute any pending delete operations.
5. Repeat.

[BLAKE3]: https://en.wikipedia.org/wiki/BLAKE_%28hash_function%29

## Index File Format

The index file is a zstd-compressed line-based text file designed to provide a simple overview of duplicate files within a directory tree. fsx does not care about the file name, but the convention is to use a `.fsidx` file extension.

The first two lines are the header consisting of the format version and the root directory that was scanned to generate the index. The root is treated as a raw string and may be empty if the index is of something other than the local file system.

The remaining lines consist of groups of files that share identical content (same digest and size). Files in each group begin with flags that describe the per-file state, followed by a path relative to the root. The first path in each group is followed by the file modification time. Subsequent files in the group may omit the modification time if it matches the predecessor. File paths may contain any valid UTF-8 byte sequence except LF, may not start with a tab, and must be slash-separated, relative, and [clean](https://pkg.go.dev/path#Clean).

Each group ends with a singe line, identified by the double tab prefix, consisting of the 256-bit BLAKE3 digest and size shared by all files in that group. If the size is 0 (empty file), then the digest is calculated from the path.

### ABNF

Index file syntax in [RFC 5234](https://datatracker.ietf.org/doc/html/rfc5234) ABNF format:

```ABNF
index      =  header *group

header     =  version LF root-path LF
version    =  "fsx index v1"           ; File format signature and version
root-path  =  *( path-step / "/" )     ; Index root path

group      =  1*( file LF ) attr LF
file       =  [ file-flag ] HTAB rel-path [ path-term [ *HTAB mtime ] ]
attr       =  2HTAB digest HTAB size

file-flag  =  "D" [ "X" ]  ; Duplicate (this copy may be removed)
file-flag  =/ "J" [ "X" ]  ; Junk (all copies may be removed)
file-flag  =/ "K" [ "X" ]  ; Keep (this copy should be preserved)
                           ; "X" indicates that file no longer exists

rel-path   =  path-step *( "/" path-step )       ; Relative UTF-8 slash-separated file path
path-step  =  1*( %x00-09 / %x0B-2E / %x30-FF )  ; Any byte except LF and "/"

path-term  =  HTAB "//"  ; Terminator added to unambiguously separate the path
                         ; from the modification time. It is also added if the
                         ; path ends with whitespace to prevent trimming it.

mtime      =  date-time  ; RFC 3339 file modification time
digest     =  64HEXDIG   ; 256-bit BLAKE3 digest
size       =  1*DIGIT    ; File size in bytes
```
