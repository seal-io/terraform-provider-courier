#!/usr/bin/env sh

set -o errexit

command_exists() {
  command -v "$@" >/dev/null 2>&1
}

version_compare() (
  set +x

  yy_a="$(echo "$1" | cut -d'.' -f1)"
  yy_b="$(echo "$2" | cut -d'.' -f1)"
  if [ "$yy_a" -lt "$yy_b" ]; then
    return 1
  fi
  if [ "$yy_a" -gt "$yy_b" ]; then
    return 0
  fi
  mm_a="$(echo "$1" | cut -d'.' -f2)"
  mm_b="$(echo "$2" | cut -d'.' -f2)"

  # Trim leading zeros to accommodate CalVer.
  mm_a="${mm_a#0}"
  mm_b="${mm_b#0}"

  if [ "${mm_a:-0}" -lt "${mm_b:-0}" ]; then
    return 1
  fi

  return 0
)

log() {
  lvl="${1}"
  shift 1

  msg="$*"

  ts="$(date +"[%m%d %H:%M:%S]")"
  echo "[${lvl}] ${ts} ${msg}" >&2

  if [ "${lvl}" = "FATAL" ]; then
    exit 1
  fi
}

root_call() {
  usr="$(id -un 2>/dev/null || true)"

  cmd='sh -c'

  if [ "${usr}" != 'root' ]; then
    if command_exists sudo; then
      cmd='sudo -E sh -c'
    elif command_exists su; then
      cmd='su -c'
    else
      log "FATAL" "User '${usr}' cannot use root_call"
    fi
  fi

  echo "${cmd}"
}

checksum() {
  archive="${1:-}"
  if [ -z "${archive}" ]; then
    log "FATAL" "Missing archive"
  fi
  shift 1

  expected_sum="${1:-}"
  if [ -z "${expected_sum}" ]; then
    log "FATAL" "Missing expected sum"
  fi

  sum_cmd=""
  case "${expected_sum}" in
  sha256:*) sum_cmd="sha256sum ${archive} | cut -d' ' -f1 | xargs -I {} echo \"sha256:{}\"" ;;
  sha224:*) sum_cmd="sha224sum ${archive} | cut -d' ' -f1 | xargs -I {} echo \"sha224:{}\"" ;;
  sha384:*) sum_cmd="sha384sum ${archive} | cut -d' ' -f1 | xargs -I {} echo \"sha384:{}\"" ;;
  sha512:*) sum_cmd="sha512sum ${archive} | cut -d' ' -f1 | xargs -I {} echo \"sha512:{}\"" ;;
  *) log "FATAL" "Unsupported sum algorithm" ;;
  esac

  actual_sum=$($(root_call) "${sum_cmd}")
  if [ "${actual_sum}" = "${expected_sum}" ]; then
    return 0
  fi

  return 1
}

uncompress() {
  archive="${1:-}"
  if [ -z "${archive}" ]; then
    log "FATAL" "Missing archive"
  fi
  shift $(( $# > 0 ? 1 : 0 ))

  dir=$(dirname "${archive}")

  uncompress_cmd=""
  case "${archive}" in
  *.tar.gz | *.tgz) uncompress_cmd="tar -xzf \"${archive}\" --directory \"${dir}\"" ;;
  *.tar.xz | *.txz) uncompress_cmd="tar -xJf \"${archive}\" --directory \"${dir}\"" ;;
  *.tar.bz2 | *.tbz2) uncompress_cmd="tar -xjf \"${archive}\" --directory \"${dir}\"" ;;
  *.tar) uncompress_cmd="tar -xf \"${archive}\" --directory \"${dir}\"" ;;
  *.gz) uncompress_cmd="gunzip -f \"${archive}\"" ;;
  *.zip) uncompress_cmd="unzip -o \"${archive}\" -d \"${dir}\"" ;;
  *) log "FATAL" "Unsupported archive '${archive}'" ;;
  esac

  $(root_call) "${uncompress_cmd}" "$@"
}

get_distro() {
  distro=""

  if [ -r /etc/os-release ]; then
    distro="$(. /etc/os-release && echo "$ID")"
  fi

  if [ -z "${distro}" ] && command_exists lsb_release; then
    set +e
    lsb_release -a -u >/dev/null 2>&1
    lsb_release_exit_code=$?
    set -e

    if [ "${lsb_release_exit_code}" = "0" ]; then
      distro=$(lsb_release -a -u 2>&1 | tr '[:upper:]' '[:lower:]' | grep -E 'id' | cut -d ':' -f 2 | tr -d '[:space:]')
    fi
  fi

  if [ "${distro}" = "osmc" ]; then
    distro="raspbian"
  fi

  echo "${distro}" | tr '[:upper:]' '[:lower:]'
}

get_distro_version() {
  distro="$(get_distro)"

  case "${distro}" in
  ubuntu)
    if command_exists lsb_release; then
      set +e
      lsb_release -a -u >/dev/null 2>&1
      lsb_release_exit_code=$?
      set -e

      if [ "${lsb_release_exit_code}" = "0" ]; then
        distro_version=$(lsb_release -a -u 2>&1 | tr '[:upper:]' '[:lower:]' | grep -E 'codename' | cut -d ':' -f 2 | tr -d '[:space:]')
      else
        distro_version="$(lsb_release --codename | cut -f2)"
      fi
    fi
    if [ -z "${distro_version}" ] && [ -r /etc/lsb-release ]; then
      distro_version="$(. /etc/lsb-release && echo "$DISTRIB_CODENAME")"
    fi
    ;;
  debian | raspbian)
    distro_version="$(sed 's/\/.*//' /etc/debian_version | sed 's/\..*//')"
    case "${distro_version}" in
    13) distro_version="trixie" ;;
    12) distro_version="bookworm" ;;
    11) distro_version="bullseye" ;;
    10) distro_version="buster" ;;
    9) distro_version="stretch" ;;
    8) distro_version="jessie" ;;
    esac
    ;;
  centos | rhel | sles)
    if [ -z "${distro_version}" ] && [ -r /etc/os-release ]; then
      distro_version="$(. /etc/os-release && echo "$VERSION_ID")"
    fi
    ;;
  *)
    if command_exists lsb_release; then
      set +e
      lsb_release -a -u >/dev/null 2>&1
      lsb_release_exit_code=$?
      set -e

      if [ "${lsb_release_exit_code}" = "0" ]; then
        distro_version=$(lsb_release -a -u 2>&1 | tr '[:upper:]' '[:lower:]' | grep -E 'codename' | cut -d ':' -f 2 | tr -d '[:space:]')
      else
        distro_version="$(lsb_release --release | cut -f2)"
      fi
    fi
    if [ -z "${distro_version}" ] && [ -r /etc/os-release ]; then
      distro_version="$(. /etc/os-release && echo "$VERSION_ID")"
    fi
    ;;
  esac
}

download() {
  uri="${1:-}"
  if [ -z "${uri}" ]; then
    log "FATAL" "Missing URL"
  fi
  shift 1

  dest="${1:-}"
  if [ -z "${dest}" ]; then
    log "FATAL" "Missing destination"
  fi
  shift $(( $# > 0 ? 1 : 0 ))

  digest="${1:-}"
  shift $(($# > 0 ? 1 : 0))

  authn_type="${1:-}"
  shift $(($# > 0 ? 1 : 0))

  authn_user="${1:-}"
  shift $(($# > 0 ? 1 : 0))

  authn_secret="${1:-}"
  shift $(($# > 0 ? 1 : 0))

  if [ -n "${digest}" ] && [ -f "${dest}" ]; then
    if checksum "${dest}" "${digest}"; then
      return 0
    fi
  fi

  rc=$(root_call)

  mkdir_cmd="mkdir -p $(dirname "${dest}")"
  ${rc} "${mkdir_cmd}"

  download_cmd=""
  if command_exists curl; then
    case "${authn_type}" in
    basic) download_cmd="curl --user \"${authn_user}:${authn_secret}\" --retry 3 --retry-all-errors --retry-delay 3 -fsSL -o \"${dest}\" \"${uri}\"" ;;
    bearer) download_cmd="curl --header \"Authorization: Bearer ${authn_secret}\" --retry 3 --retry-all-errors --retry-delay 3 -fsSL -o \"${dest}\" \"${uri}\"" ;;
    *) download_cmd="curl --retry 3 --retry-delay 3 -fsSL -o \"${dest}\" \"${uri}\"" ;;
    esac
  elif command_exists wget; then
    case "${authn_type}" in
    basic) download_cmd="wget --http-user \"${authn_user}\" --http-password \"${authn_secret}\" --retry-connrefused --waitretry=3 --tries=3 --continue --no-check-certificate -O \"${dest}\" \"${uri}\"" ;;
    bearer) download_cmd="wget --header \"Authorization: Bearer ${authn_secret}\" --retry-connrefused --waitretry=3 --tries=3 --continue --no-check-certificate -O \"${dest}\" \"${uri}\"" ;;
    *) download_cmd="wget --retry-connrefused --waitretry=3 --tries=3 --continue --no-check-certificate -O \"${dest}\" \"${uri}\"" ;;
    esac
  else
    log "FATAL" "No curl or wget available"
  fi
  ${rc} "${download_cmd}"

  if [ -n "${digest}" ] && [ -f "${dest}" ]; then
    if ! checksum "${dest}" "${digest}"; then
      log "FATAL" "Corrupted downloaded"
      exit 1
    fi
  fi
}
