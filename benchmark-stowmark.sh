#!/usr/bin/env bash
set -Eeuo pipefail

# Resultats numerics estables i CSV valid independentment de la configuracio regional.
export LC_ALL=C

# Benchmark end-to-end de Stowmark.
# Configuracio per variables d'entorn; exemples:
#   STOWMARK_BIN="$PWD/stowmark" ./benchmark-stowmark.sh
#   REPOSITORY='webdav://localhost:18080/backups/benchmark' ./benchmark-stowmark.sh
#   SMALL_FILES=10000 SMALL_SIZE_KIB=16 LARGE_FILES=10 LARGE_SIZE_MIB=1024 ./benchmark-stowmark.sh
#   ENCRYPTION=0 COMPRESSION=none ./benchmark-stowmark.sh

STOWMARK_BIN="${STOWMARK_BIN:-./stowmark}"
RESULTS_ROOT="${RESULTS_ROOT:-./benchmark-results}"
SMALL_FILES="${SMALL_FILES:-1000}"
SMALL_SIZE_KIB="${SMALL_SIZE_KIB:-16}"
LARGE_FILES="${LARGE_FILES:-4}"
LARGE_SIZE_MIB="${LARGE_SIZE_MIB:-64}"
COMPRESSION="${COMPRESSION:-zstd}"
COMPRESSION_LEVEL="${COMPRESSION_LEVEL:-3}"
ENCRYPTION="${ENCRYPTION:-1}"
REPOSITORY="${REPOSITORY:-}"

for dependency in "$STOWMARK_BIN" diff find grep head awk date du; do
  if [[ "$dependency" == "$STOWMARK_BIN" ]]; then
    [[ -x "$dependency" ]] || { printf 'No es pot executar STOWMARK_BIN=%s\n' "$dependency" >&2; exit 1; }
  else
    command -v "$dependency" >/dev/null || { printf 'Falta la dependencia: %s\n' "$dependency" >&2; exit 1; }
  fi
done

for value in SMALL_FILES SMALL_SIZE_KIB LARGE_FILES LARGE_SIZE_MIB; do
  [[ "${!value}" =~ ^[0-9]+$ ]] || { printf '%s ha de ser un enter no negatiu\n' "$value" >&2; exit 1; }
done

run_id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
run_dir="$RESULTS_ROOT/$run_id"
origin_1="$run_dir/origin-1"
origin_2="$run_dir/origin-2"
repository="${REPOSITORY:-$run_dir/repository}"
restore_dir="$run_dir/restore"
keys_dir="$run_dir/keys"
logs_dir="$run_dir/logs"
csv="$run_dir/timings.csv"
summary="$run_dir/summary.txt"

mkdir -p "$origin_1/small" "$origin_1/large" "$origin_2/small" "$origin_2/large" "$keys_dir" "$logs_dir"
printf 'operation,status,duration_seconds\n' > "$csv"

generate_origin() {
  local destination="$1" seed="$2" i

  # Fitxers petits compressibles, amb contingut diferent per origen.
  for ((i = 1; i <= SMALL_FILES; i++)); do
    awk -v seed="$seed" -v file_index="$i" -v bytes="$((SMALL_SIZE_KIB * 1024))" \
      'BEGIN { line=sprintf("stowmark benchmark origin=%s file=%08d ", seed, file_index); while (written < bytes) { remaining=bytes-written; out=(remaining < length(line) ? substr(line,1,remaining) : line); printf "%s", out; written+=length(out) } }' \
      > "$destination/small/file-$(printf '%08d' "$i").txt"
  done

  # Fitxers grans no compressibles per exercitar chunking, hash, xifrat i E/S.
  for ((i = 1; i <= LARGE_FILES; i++)); do
    dd if=/dev/urandom of="$destination/large/file-$(printf '%04d' "$i").bin" \
      bs=1M count="$LARGE_SIZE_MIB" status=none
  done
}

measure() {
  local name="$1" start_ns end_ns duration status=ok
  shift
  start_ns="$(date +%s%N)"

  if "$@" >"$logs_dir/$name.log" 2>&1; then
    :
  else
    status=failed
  fi

  end_ns="$(date +%s%N)"
  duration="$(awk -v start="$start_ns" -v end="$end_ns" 'BEGIN { printf "%.6f", (end-start)/1000000000 }')"
  printf '%s,%s,%s\n' "$name" "$status" "$duration" | tee -a "$csv"

  if [[ "$status" == failed ]]; then
    printf '\nHa fallat %s. Log: %s\n' "$name" "$logs_dir/$name.log" >&2
    return 1
  fi
}

latest_snapshot_id() {
  local snapshot_id

  if ! "$STOWMARK_BIN" snapshot list --repo "$repository" >"$logs_dir/snapshot_list_latest.log" 2>&1; then
    printf 'No s\x27ha pogut consultar la llista de snapshots. Log: %s\n' "$logs_dir/snapshot_list_latest.log" >&2
    return 1
  fi

  snapshot_id="$(grep -Eo '[0-9]{8}T[0-9]{6}Z-[[:xdigit:]]+' "$logs_dir/snapshot_list_latest.log" | head -n 1 || true)"
  [[ -n "$snapshot_id" ]] || { printf 'No s\x27ha pogut extraure cap ID de snapshot. Log: %s\n' "$logs_dir/snapshot_list_latest.log" >&2; return 1; }

  printf '%s\n' "$snapshot_id"
}

printf 'Generant origin 1...\n'
generate_origin "$origin_1" one
printf 'Generant origin 2...\n'
generate_origin "$origin_2" two

init_args=(init --repo "$repository" --compression "$COMPRESSION")
if [[ "$COMPRESSION" != none ]]; then
  init_args+=(--level "$COMPRESSION_LEVEL")
fi

private_key_args=()
if [[ "$ENCRYPTION" == 1 ]]; then
  "$STOWMARK_BIN" key generate --folder "$keys_dir" >"$logs_dir/key_generate.log" 2>&1

  private_key="$(find "$keys_dir" -maxdepth 1 -type f -name '*-private.pem' -print -quit)"
  public_key="$(find "$keys_dir" -maxdepth 1 -type f -name '*-public.pem' -print -quit)"

  [[ -n "$private_key" ]] || { printf 'No s\x27ha trobat la clau privada generada dins %s\n' "$keys_dir" >&2; exit 1; }
  [[ -n "$public_key" ]] || { printf 'No s\x27ha trobat la clau publica generada dins %s\n' "$keys_dir" >&2; exit 1; }

  init_args+=(--public-key "$public_key")
  private_key_args=(--private-key "$private_key")
elif [[ "$ENCRYPTION" != 0 ]]; then
  printf 'ENCRYPTION ha de ser 0 o 1\n' >&2
  exit 1
fi

measure init "$STOWMARK_BIN" "${init_args[@]}"
measure snapshot_create_origin_1 "$STOWMARK_BIN" snapshot create "$origin_1" --repo "$repository" "${private_key_args[@]}"
first_snapshot_id="$(latest_snapshot_id)"

measure snapshot_create_unchanged "$STOWMARK_BIN" snapshot create "$origin_1" --repo "$repository" "${private_key_args[@]}"
unchanged_snapshot_id="$(latest_snapshot_id)"

measure snapshot_create_origin_2 "$STOWMARK_BIN" snapshot create "$origin_2" --repo "$repository" "${private_key_args[@]}"
second_snapshot_id="$(latest_snapshot_id)"

measure snapshot_verify "$STOWMARK_BIN" snapshot verify --id "$second_snapshot_id" --repo "$repository" "${private_key_args[@]}"
measure snapshot_get "$STOWMARK_BIN" snapshot get --id "$second_snapshot_id" --repo "$repository"
measure snapshot_restore "$STOWMARK_BIN" snapshot restore --id "$second_snapshot_id" --repo "$repository" --destination "$restore_dir" "${private_key_args[@]}"
measure restore_diff diff -qr "$origin_2" "$restore_dir"

if [[ "$ENCRYPTION" == 1 ]]; then
  measure key_rekey "$STOWMARK_BIN" key rekey --repo "$repository" --private-key "$private_key" --public-key "$public_key"
  measure snapshot_verify_after_rekey "$STOWMARK_BIN" snapshot verify --id "$second_snapshot_id" --repo "$repository" --private-key "$private_key"
else
  printf 'key_rekey,skipped,0.000000\n' | tee -a "$csv"
  printf 'snapshot_verify_after_rekey,skipped,0.000000\n' | tee -a "$csv"
fi

origin_1_bytes="$(du -sb "$origin_1" | awk '{print $1}')"
origin_2_bytes="$(du -sb "$origin_2" | awk '{print $1}')"
if [[ -d "$repository" ]]; then
  repository_bytes="$(du -sb "$repository" | awk '{print $1}')"
else
  repository_bytes="not_available_for_remote_repository"
fi

{
  printf 'run_id=%s\n' "$run_id"
  printf 'stowmark_bin=%s\n' "$STOWMARK_BIN"
  printf 'repository=%s\n' "$repository"
  printf 'compression=%s\n' "$COMPRESSION"
  printf 'encryption=%s\n' "$ENCRYPTION"
  printf 'small_files_per_origin=%s\n' "$SMALL_FILES"
  printf 'small_size_kib=%s\n' "$SMALL_SIZE_KIB"
  printf 'large_files_per_origin=%s\n' "$LARGE_FILES"
  printf 'large_size_mib=%s\n' "$LARGE_SIZE_MIB"
  printf 'origin_1_bytes=%s\n' "$origin_1_bytes"
  printf 'origin_2_bytes=%s\n' "$origin_2_bytes"
  printf 'repository_bytes=%s\n' "$repository_bytes"
  printf 'first_snapshot_id=%s\n' "$first_snapshot_id"
  printf 'unchanged_snapshot_id=%s\n' "$unchanged_snapshot_id"
  printf 'second_snapshot_id=%s\n' "$second_snapshot_id"
  printf 'timings_csv=%s\n' "$csv"
} >> "$summary"

printf '\nProva completada.\nResultats: %s\nResum: %s\nLogs: %s\n' "$csv" "$summary" "$logs_dir"
