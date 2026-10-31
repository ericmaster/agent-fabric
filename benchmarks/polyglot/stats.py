#!/usr/bin/env python3
"""Usage: stats.py results.csv  -> per-arm stats + pi/fabric restricted to pipeline's tasks"""
import csv,sys,statistics as st
R=list(csv.DictReader(open(sys.argv[1])))
bad=[r for r in R if int(r["pass2"])<int(r["pass1"])]
print("pass2<pass1 rows (should be 0 => cumulative):",len(bad))
def tab(rows,title):
    print(f"\n### {title}\n| arm | n | pass@1 | pass@2 | avg wall s | median wall s | avg total tok | rows w/ errors | terminated rows |\n|---|---|---|---|---|---|---|---|---|")
    for a in ("pi","fabric","pipeline"):
        x=[r for r in rows if r["arm"]==a]
        if not x: continue
        n=len(x); p1=sum(int(r["pass1"]) for r in x); p2=sum(int(r["pass2"]) for r in x); w=[float(r["wall_s"]) for r in x]
        print(f"| {a} | {n} | {p1}/{n} ({100*p1/n:.0f}%) | {p2}/{n} ({100*p2/n:.0f}%) | {st.mean(w):.0f} | {st.median(w):.0f} | {st.mean(int(r['total_tokens']) for r in x):,.0f} | {sum(bool(r['errors']) for r in x)} | {sum('terminated' in r['errors'] for r in x)} |")
tab(R,"All completed rows")
pt={(r["lang"],r["task"]) for r in R if r["arm"]=="pipeline"}
tab([r for r in R if (r["lang"],r["task"]) in pt],f"Same {len(pt)} tasks pipeline completed")
for r in R:
    if r["task"]=="meetup" and r["arm"]=="fabric": print("\njavascript-meetup-fabric:",r)
