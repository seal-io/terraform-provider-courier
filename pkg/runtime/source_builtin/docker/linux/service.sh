#!/usr/bin/env sh

set -o errexit

# shellcheck disable=SC1007
ROOT_DIR=$(CDPATH= cd "$(dirname "$0")/../.." && pwd -P)
for file in "${ROOT_DIR}/lib/linux/"*; do
  if [ -f "${file}" ]; then
    # shellcheck disable=SC1090
    . "${file}"
  fi
done

COURIER_PATH="/var/local/courier/artifact"

#
# Stages
#

## service.sh setup "${artifact_id}" "${refer_uri}" "${refer_digest}" "${refer_authn_type}" "${refer_authn_user}" "${refer_authn_secret}"
setup() {
  art="${1:-}"
  if [ -z "${art}" ]; then
    log "FATAL" "Missing artifact"
  fi
  shift 1

  refer_uri="${1:-}"
  shift $(($# > 0 ? 1 : 0))

  refer_digest="${1:-}"
  shift $(($# > 0 ? 1 : 0))

  refer_authn_type="${1:-}"
  shift $(($# > 0 ? 1 : 0))

  refer_authn_user="${1:-}"
  shift $(($# > 0 ? 1 : 0))

  refer_authn_secret="${1:-}"
  shift $(($# > 0 ? 1 : 0))

  rc=$(root_call)

  ##
  ## Install
  ##
  if ! command_exists docker; then
    uri="https://get.docker.com"
    dest="/tmp/get-docker.sh"
    download "${uri}" "${dest}"
    ${rc} "chmod a+x ${dest}"
    ${rc} "${dest} --mirror Aliyun --version 23.0"
  fi

  if [ "${COURIER_DEPENDENT:-"false"}" = "true" ]; then
    return 0
  fi

  ##
  ## Download
  ##
  dck_img=$(echo "${refer_uri}" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')
  if [ -z "${dck_img}" ]; then
    log "FATAL" "Missing docker image"
  fi
  if [ -n "${refer_authn_user}" ] && [ -n "${refer_authn_secret}" ]; then
    dck_reg="index.docker.io"
    if [ -n "$(echo "${dck_img}" | cut -d'/' -f3)" ]; then
      dck_reg="$(echo "${dck_img}" | cut -d'/' -f1)"
    fi
    ${rc} "docker login --username ${refer_authn_user} --password ${refer_authn_secret} ${dck_reg}"
  fi
  ${rc} "docker pull --quiet ${dck_img}"

  ##
  ## Prepare
  ##
  dck_cmd="docker create --restart always --name ${art}"

  if [ -f "${COURIER_PATH}/${art}/ports" ]; then
    while read -r port; do
      if [ -n "${port}" ]; then
        dck_cmd="${dck_cmd} --publish ${port}:${port}"
      fi
    done <"${COURIER_PATH}/${art}/ports"
  fi

  if [ -f "${COURIER_PATH}/${art}/envs" ]; then
    while read -r env; do
      if [ -n "${env}" ]; then
        dck_cmd="${dck_cmd} --env ${env}"
      fi
    done <"${COURIER_PATH}/${art}/envs"
  fi

  if [ -f "${COURIER_PATH}/${art}/volumes" ]; then
    while read -r volume; do
      if [ -n "${volume}" ]; then
        dck_cmd="${dck_cmd} --volume ${volume}"
      fi
    done <"${COURIER_PATH}/${art}/volumes"
  fi

  dck_cmd="${dck_cmd} ${dck_img}"

  if [ -f "${COURIER_PATH}/${art}/command" ]; then
    command=$(cat <"${COURIER_PATH}/${art}/command" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')
    if [ -n "${command}" ]; then
      dck_cmd="${dck_cmd} ${command}"
    fi
  fi

  ##
  ## Create
  ##
  ${rc} "${dck_cmd}"
}

## service.sh start "${artifact_id}"
start() {
  art="${1:-}"
  if [ -z "${art}" ]; then
    log "FATAL" "Missing artifact"
  fi

  rc=$(root_call)

  ${rc} "docker start ${art}"
}

## service.sh state "${artifact_id}"
state() {
  art="${1:-}"
  if [ -z "${art}" ]; then
    log "FATAL" "Missing artifact"
  fi

  rc=$(root_call)

  ${rc} "docker inspect ${art}"
}

## service.sh stop "${artifact_id}"
stop() {
  art="${1:-}"
  if [ -z "${art}" ]; then
    log "FATAL" "Missing artifact"
  fi

  rc=$(root_call)

  ${rc} "docker stop ${art}"
}

## service.sh cleanup "${artifact_id}"
cleanup() {
  art="${1:-}"
  if [ -z "${art}" ]; then
    log "FATAL" "Missing artifact"
  fi

  rc=$(root_call)

  ${rc} "docker remove --force --volumes ${art}"
}

#
# Entry
#

entry() {
  stage="${1:-}"
  shift $(($# > 0 ? 1 : 0))

  case "${stage}" in
  setup) setup "$@" ;;
  start) start "$@" ;;
  state) state "$@" ;;
  stop) stop "$@" ;;
  cleanup) cleanup "$@" ;;
  *) log "FATAL" "Unsupported stage '${stage}'" ;;
  esac
}

entry "$@"
