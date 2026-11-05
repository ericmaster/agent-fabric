# Router por dificultad (primer corte)

Código: [`internal/router`](../internal/router/router.go), CLI `agent-fabric route`.

## Problema
En el benchmark Polyglot (`agent-fabric-bench`, `results/run50-luna`, 50 tareas) el pipeline
no aporta en tareas fáciles: `pi` directo 35/50 pass2, `fabric` 32/50, `pipeline` completo
10/16 con ~268k tokens y ~2700 s por tarea (vs ~20k–51k tokens, 120–200 s). Fabric solo ganó en 3
tareas y perdió en 4. Conclusión: por defecto ir directo; pagar el pipeline solo cuando hay señales.

## Rutas
- **easy** → `implementor`.
- **complex** → `planner` → `implementor` → `code-reviewer`.

## Señales (heurística, sin LLM)
Cada una suma 1 punto; `score >= 2` ⇒ complex.
- Longitud: más de 120 palabras.
- Archivos esperados (`--files`): más de 2.
- Palabras clave como prefijo de palabra (refactor, migrat, architect, schema, security,
  authentica, concurren, rollback, integration, distributed…): 1 punto c/u, máximo 2.
- Tarea vacía ⇒ complex (fail-safe).

Una sola señal no basta: los ejercicios Polyglot son enunciados largos de 1 archivo y deben ir directo.

## Override y fallback
- `--difficulty easy|complex` fuerza la ruta (gana sobre la heurística).
- Si el implementor falla en una tarea easy (tests rojos, error), el orquestador llama
  `--escalate "<motivo>"` y re-ejecuta con el pipeline completo. Escalar complex es no-op;
  `--escalate` con `--difficulty easy` es error. Flags van antes del texto; stdin (`-`) ≤ 1 MiB.

## Extensión futura
Llamada LLM barata (clasificador) solo en la zona gris (`score == 1`); no implementada.

## Medición con el bench
Nuevo brazo `router` en `agent-fabric-bench`: por tarea registrar `difficulty`, `escalated`,
`pass2`, `wall_s`, `total_tokens`. Éxito si `router` ≥ `pi` en pass2 y ≤ `fabric` en tokens;
revisar falsos easy (escalaciones) y falsos complex (pipeline sin mejora) para ajustar umbrales.
