#!/usr/bin/env python3
"""Генерирует migrations/data/cities_ru.csv из дампов GeoNames (CC BY 4.0):
  cities15000.txt  — города с населением > 15 000
  RU.txt           — alternatenames/RU.zip, русские названия (isolanguage=ru)
  admin1CodesASCII.txt — коды регионов (названия берутся из тех же русских alternatenames)
Использование: gen_cities.py <cities15000.txt> <alternatenames_RU.txt> <admin1CodesASCII.txt> <out.csv>"""
import csv, sys

ru = {}
for line in open(sys.argv[2], encoding="utf-8"):
    f = line.rstrip("\n").split("\t")
    if len(f) < 4 or f[2] != "ru": continue
    gid, name = f[1], f[3]
    pref = len(f) > 4 and f[4] == "1"
    short = len(f) > 5 and f[5] == "1"
    coll = len(f) > 6 and f[6] == "1"
    hist = len(f) > 7 and f[7] == "1"
    if (coll and not pref) or hist or name.startswith('г. ') or name in ('СПб', 'Питер'): continue
    score = (1 if pref else 0, 0)
    if gid not in ru or score > ru[gid][0]:
        ru[gid] = (score, name)

OVERRIDES = {'498817': 'Санкт-Петербург'}
ru.update({k: (9, v) for k, v in OVERRIDES.items()})

admin = {}
for line in open(sys.argv[3], encoding='utf-8'):
    f = line.rstrip('\n').split('\t')
    if f[0].startswith('RU.') and f[3] in ru:
        admin[f[0][3:]] = ru[f[3]][1]

CRIMEA = {
    'Sevastopol': ('Севастополь', 'Севастополь'),
    'Simferopol': ('Симферополь', 'Республика Крым'),
    'Kerch': ('Керчь', 'Республика Крым'),
    'Yevpatoriya': ('Евпатория', 'Республика Крым'),
    'Yalta': ('Ялта', 'Республика Крым'),
    'Feodosiya': ('Феодосия', 'Республика Крым'),
    'Dzhankoy': ('Джанкой', 'Республика Крым'),
    'Alushta': ('Алушта', 'Республика Крым'),
    'Saky': ('Саки', 'Республика Крым'),
    'Bakhchysaray': ('Бахчисарай', 'Республика Крым'),
    'Armyansk': ('Армянск', 'Республика Крым'),
    'Sudak': ('Судак', 'Республика Крым'),
    'Bilohirsk': ('Белогорск', 'Республика Крым'),
    'Yany Kapu': ('Красноперекопск', 'Республика Крым'),
}

rows = []
for line in open(sys.argv[1], encoding="utf-8"):
    f = line.rstrip("\n").split("\t")
    if f[8] == "UA" and f[17] == "Europe/Simferopol" and f[2] in CRIMEA:
        name, region = CRIMEA[f[2]]
        rows.append((name, region, "Europe/Moscow", int(f[14] or 0), f[4], f[5]))
        continue
    if f[8] != "RU" or f[0] not in ru: continue
    rows.append((ru[f[0]][1], admin.get(f[10], ''), f[17], int(f[14] or 0), f[4], f[5]))
import re
_generic = re.compile(r"(?<=\S )(Область|Край|Округ|Автономный|Автономная)\b")
rows = [(n, _generic.sub(lambda m: m.group(1).lower(), reg), tz, pop, lat, lon) for n, reg, tz, pop, lat, lon in rows]
rows.sort(key=lambda r: -r[3])
with open(sys.argv[4], "w", encoding="utf-8", newline="") as fh:
    w = csv.writer(fh); w.writerow(["name","region","timezone","population","lat","lon"]); w.writerows(rows)
print(len(rows), "cities")
