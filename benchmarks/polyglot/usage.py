#!/usr/bin/env python3
"""Usage: usage.py events.jsonl -> input,output,cacheRead,cacheWrite,totalTokens summed over assistant messages"""
import json,sys
k=["input","output","cacheRead","cacheWrite","totalTokens"]; s=dict.fromkeys(k,0)
for line in open(sys.argv[1],errors="ignore"):
    try: e=json.loads(line)
    except Exception: continue
    m=e.get("message") or {}
    if e.get("type")=="message_end" and m.get("role")=="assistant":
        u=m.get("usage") or {}
        for x in k: s[x]+=u.get(x,0) or 0
print(",".join(str(s[x]) for x in k))
