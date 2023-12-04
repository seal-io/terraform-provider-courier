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

SYSTEMD_PATH="/etc/systemd/system"

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
  if ! command_exists java; then
    distro="$(get_distro)"
    case "${distro}" in
    ubuntu | debian | raspbian)
      ${rc} "DEBIAN_FRONTEND=noninteractive apt update -y"
      ${rc} "DEBIAN_FRONTEND=noninteractive apt install -y openjdk-18-jre-headless"
      ;;
    centos | fedora | rhel)
      ${rc} "yum update -y"
      ${rc} "yum install -y java-18-openjdk"
      ;;
    sles)
      ${rc} "zypper update -y"
      ${rc} "zypper install -y java-18-openjdk"
      ;;
    *) log "FATAL" "Unsupported distro '${distro}'" ;;
    esac
  fi

  if [ "${COURIER_DEPENDENT:-"false"}" = "true" ]; then
    return 0
  fi

  ##
  ## Download
  ##
  uri=$(echo "${refer_uri}" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')
  if [ -z "${uri}" ]; then
    log "FATAL" "Missing refer URI"
  fi
  dest="${COURIER_PATH}/${art}/target.jar"
  download "${uri}" "${dest}" "${refer_digest}" "${refer_authn_type}" "${refer_authn_user}" "${refer_authn_secret}"

  ##
  ## Prepare
  ##
  ${rc} "chown -R $(id -u):$(id -g) ${COURIER_PATH}/${art}"

  mkdir "${COURIER_PATH}/${art}/bin"
  cat <<EOF >"${COURIER_PATH}/${art}/bin/startup.sh"
#!/bin/sh

command=""
if [ -f "${COURIER_PATH}/${art}/command" ]; then
  command=$(cat <"${COURIER_PATH}/${art}/command" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')
fi
java -jar ${COURIER_PATH}/${art}/target.jar "\${command}"
EOF
  cat <<EOF >"${COURIER_PATH}/${art}/bin/shutdown.sh"
#!/bin/sh

jps -lv | grep ${COURIER_PATH}/${art}/target.jar | awk '{print \$1}' | xargs kill -15
EOF
  chmod a+x "${COURIER_PATH}/${art}/bin/"*.sh

  ##
  ## Create
  ##
  reload="n"
  if [ -e "${SYSTEMD_PATH}/openjdk-${art}.service" ]; then
    reload="y"
  fi
  cat <<EOF >"${COURIER_PATH}/${art}/openjdk.service"
[Unit]
Description=OpenJDK-${art}
After=syslog.target network.target

[Install]
WantedBy=multi-user.target

[Service]
Type=simple

Restart=on-failure
RestartSec=10

ExecStart=${COURIER_PATH}/${art}/bin/startup.sh
ExecStop=${COURIER_PATH}/${art}/bin/shutdown.sh

EnvironmentFile=${COURIER_PATH}/${art}/envs
EOF
  if [ ! -e "${SYSTEMD_PATH}/openjdk-${art}.service" ]; then
    ${rc} "mkdir -p ${SYSTEMD_PATH}"
    ${rc} "ln -s ${COURIER_PATH}/${art}/openjdk.service ${SYSTEMD_PATH}/openjdk-${art}.service"
  fi

  if [ "${reload}" = "y" ]; then
    ${rc} "systemctl daemon-reload || true"
  fi
}

## service.sh start "${artifact_id}"
start() {
  art="${1:-}"
  if [ -z "${art}" ]; then
    log "FATAL" "Missing artifact"
  fi

  rc=$(root_call)

  ${rc} "systemctl start openjdk-${art}.service"
}

## service.sh state "${artifact_id}"
state() {
  art="${1:-}"
  if [ -z "${art}" ]; then
    log "FATAL" "Missing artifact"
  fi

  rc=$(root_call)

  ${rc} "systemctl status openjdk-${art}.service"
}

## service.sh stop "${artifact_id}"
stop() {
  art="${1:-}"
  if [ -z "${art}" ]; then
    log "FATAL" "Missing artifact"
  fi

  rc=$(root_call)

  ${rc} "systemctl stop openjdk-${art}.service"
}

## service.sh cleanup "${artifact_id}"
cleanup() {
  art="${1:-}"
  if [ -z "${art}" ]; then
    log "FATAL" "Missing artifact"
  fi

  if [ ! -e "${SYSTEMD_PATH}/openjdk-${art}.service" ]; then
    return 0
  fi

  rc=$(root_call)

  ${rc} "systemctl stop openjdk-${art}.service || true"
  ${rc} "rm -f ${SYSTEMD_PATH}/openjdk-${art}.service"
  ${rc} "rm -rf ${COURIER_PATH}/${art}"
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
