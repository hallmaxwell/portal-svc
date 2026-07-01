import re
import sys

def process_file(filepath):
    with open(filepath, 'r') as f:
        content = f.read()

    # Replace duplicate flag error handling
    flag_err_pattern = r'if errors\.Is\(err, flag\.ErrHelp\) \{\n\s*os\.Exit\(0\)\n\s*\} else if err != nil \{\n\s*os\.Exit\(1\)\n\s*\}'
    content = re.sub(flag_err_pattern, 'shared.HandleFlagError(err)', content)

    # Replace block with err already declared: if err != nil { fmt.Fprintf... os.Exit(1) }
    err_var_pattern = r'if err != nil \{\n\s*fmt\.Fprintf\(os\.Stderr, "(.*?)%v\\n", (.*?)\)\n\s*os\.Exit\(1\)\n\s*\}'
    content = re.sub(err_var_pattern, r'shared.CheckError(err, "\1%v", \2)', content)

    # Replace block with err declaration in if: if err := func(); err != nil { fmt.Fprintf... os.Exit(1) }
    err_check_pattern = r'if err := (.*?); err != nil \{\n\s*fmt\.Fprintf\(os\.Stderr, "(.*?)%v\\n", (.*?)\)\n\s*os\.Exit\(1\)\n\s*\}'
    content = re.sub(err_check_pattern, r'shared.CheckError(\1, "\2%v")', content)

    with open(filepath, 'w') as f:
        f.write(content)

process_file('cmd/local/main.go')
process_file('cmd/remote/main.go')
