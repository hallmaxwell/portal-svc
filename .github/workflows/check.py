import os
import json
import sys

def is_raw_json(val):
    val = val.strip()
    if val.isdigit(): 
        return True
    if val.lower() in ['true', 'false']: 
        return True
    if val.startswith('[') and val.endswith(']'): 
        return True
    if val.startswith('{') and val.endswith('}'): 
        return True
    try:
        float(val)
        return True
    except ValueError:
        pass
    return False

if len(sys.argv) != 3:
    sys.exit(1)

template_path = sys.argv[1]
output_path = sys.argv[2]

with open(template_path, 'r') as f:
    content = f.read()
    
for key, val in os.environ.items():
    if not val: 
        continue
    val = val.strip('\"\'')
    
    if is_raw_json(val):
        content = content.replace(f'\"{{{key}}}\"', val)
        content = content.replace(f'{{{key}}}', val)
    else:
        content = content.replace(f'{{{key}}}', val)

try:
    data = json.loads(content)
except json.JSONDecodeError:
    sys.exit(1)
    
for inbound in data.get('inbounds', []):
    if inbound.get('type') == 'tun':
        inbound['auto_route'] = False
        inbound['strict_route'] = False
        
if 'route' in data and 'rule_set' in data['route']:
    for rs in data['route']['rule_set']:
        rs['download_detour'] = 'direct'
        
with open(output_path, 'w') as f:
    json.dump(data, f, indent=2)