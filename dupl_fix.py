import re
import sys

def process_file(filepath):
    with open(filepath, 'r') as f:
        content = f.read()

    # We will deduplicate some very obvious and safe blocks

    # Replace duplicate flag error handling
    flag_err_pattern = r'if errors\.Is\(err, flag\.ErrHelp\) \{\n\s*os\.Exit\(0\)\n\s*\} else if err != nil \{\n\s*os\.Exit\(1\)\n\s*\}'
    content = re.sub(flag_err_pattern, 'shared.HandleFlagError(err)', content)

    # In internal/app/app.go flag errors uses == not errors.Is
    flag_err_app = r'if err == flag\.ErrHelp \{\n\s*os\.Exit\(0\)\n\s*\} else if err != nil \{\n\s*os\.Exit\(1\)\n\s*\}'
    content = re.sub(flag_err_app, 'shared.HandleFlagError(err)', content)

    with open(filepath, 'w') as f:
        f.write(content)

for f in ['cmd/local/main.go', 'cmd/remote/main.go']:
    process_file(f)
