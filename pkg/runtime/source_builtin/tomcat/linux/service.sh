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
    ${rc} "chmod a+x ${ROOT_DIR}/openjdk/linux/service.sh"
    COURIER_DEPENDENT=true "${ROOT_DIR}/openjdk/linux/service.sh" setup "${art}"
  fi

  uri="https://archive.apache.org/dist/tomcat/tomcat-10/v10.1.16/bin/apache-tomcat-10.1.16.tar.gz"
  dest="/tmp/apache-tomcat-10.1.16.tar.gz"
  download "${uri}" "${dest}" "sha512:d469d0c68cf5e321bbc264c3148d28899e320942f34636e0aff3d79fc43e8472cd0420d0d3df5ef6ece4be4810a3f8fd518f605c5a9c13cac4e8f96f5f138e92"
  if [ ! -e "${COURIER_PATH}/${art}/apache-tomcat-10.1.16" ]; then
    ### Copy
    ${rc} "cp ${dest} ${COURIER_PATH}/${art}/"
    uncompress "${COURIER_PATH}/${art}/apache-tomcat-10.1.16.tar.gz"
    ${rc} "ln -s ${COURIER_PATH}/${art}/apache-tomcat-10.1.16 ${COURIER_PATH}/${art}/tomcat"
    ### Clean
    ${rc} "rm -rf ${COURIER_PATH}/${art}/tomcat/webapps/manager"
    ${rc} "rm -rf ${COURIER_PATH}/${art}/tomcat/webapps/host-manager"
    ${rc} "rm -rf ${COURIER_PATH}/${art}/tomcat/webapps/examples"
    ${rc} "rm -rf ${COURIER_PATH}/${art}/tomcat/webapps/docs"
    ${rc} "rm -rf ${COURIER_PATH}/${art}/tomcat/webapps/ROOT"
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
  dest="${COURIER_PATH}/${art}/tomcat/webapps/ROOT.war"
  download "${uri}" "${dest}" "${refer_digest}" "${refer_authn_type}" "${refer_authn_user}" "${refer_authn_secret}"

  ##
  ## Prepare
  ##
  ${rc} "chown -R $(id -u):$(id -g) ${COURIER_PATH}/${art}"
  chmod a+x "${COURIER_PATH}/${art}/tomcat/bin/"*.sh

  cat <<EOF >"${COURIER_PATH}/${art}/tomcat/conf/server.xml"
<?xml version="1.0" encoding="UTF-8"?>
<Server port="-1" shutdown="SHUTDOWN">
  <Listener className="org.apache.catalina.startup.VersionLoggerListener" />
  <Listener className="org.apache.catalina.core.AprLifecycleListener" SSLEngine="on" />
  <Listener className="org.apache.catalina.core.JreMemoryLeakPreventionListener" />
  <Listener className="org.apache.catalina.mbeans.GlobalResourcesLifecycleListener" />
  <Listener className="org.apache.catalina.core.ThreadLocalLeakPreventionListener" />
  <Service name="Catalina">
EOF

  if [ -f "${COURIER_PATH}/${art}/ports" ]; then
    while read -r port; do
      if [ -n "${port}" ]; then
        cat <<EOF >>"${COURIER_PATH}/${art}/tomcat/conf/server.xml"
            <Connector port="${port}" maxThreads="1000" protocol="HTTP/1.1" connectionTimeout="20000" />
EOF
      fi
    done <"${COURIER_PATH}/${art}/ports"
  fi

  cat <<EOF >>"${COURIER_PATH}/${art}/tomcat/conf/server.xml"
    <Engine name="Catalina" defaultHost="localhost">
      <Host name="localhost"  appBase="webapps" unpackWARs="true" autoDeploy="true" />
    </Engine>
  </Service>
</Server>
EOF

  ##
  ## Create
  ##
  reload="n"
  if [ -e "${SYSTEMD_PATH}/tomcat-${art}.service" ]; then
    reload="y"
  fi
  cat <<EOF >"${COURIER_PATH}/${art}/tomcat.service"
[Unit]
Description=Tomcat-${art}
After=syslog.target network.target

[Install]
WantedBy=multi-user.target

[Service]
Type=forking

Restart=on-failure
RestartSec=10

ExecStart=${COURIER_PATH}/${art}/tomcat/bin/startup.sh
ExecStop='${COURIER_PATH}/${art}/tomcat/bin/shutdown.sh 15 -force'

EnvironmentFile=${COURIER_PATH}/${art}/envs
Environment="CATALINA_BASE=${COURIER_PATH}/${art}/tomcat"
Environment="CATALINA_HOME=${COURIER_PATH}/${art}/tomcat"
Environment="CATALINA_PID=${COURIER_PATH}/${art}/tomcat/temp/tomcat.pid"
EOF
  if [ ! -e "${SYSTEMD_PATH}/tomcat-${art}.service" ]; then
    ${rc} "mkdir -p ${SYSTEMD_PATH}"
    ${rc} "ln -s ${COURIER_PATH}/${art}/tomcat.service ${SYSTEMD_PATH}/tomcat-${art}.service"
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

  ${rc} "systemctl start tomcat-${art}.service"
}

## service.sh state "${artifact_id}"
state() {
  art="${1:-}"
  if [ -z "${art}" ]; then
    log "FATAL" "Missing artifact"
  fi

  rc=$(root_call)

  ${rc} "systemctl status tomcat-${art}.service"
}

## service.sh stop "${artifact_id}"
stop() {
  art="${1:-}"
  if [ -z "${art}" ]; then
    log "FATAL" "Missing artifact"
  fi

  rc=$(root_call)

  ${rc} "systemctl stop tomcat-${art}.service"
}

## service.sh cleanup "${artifact_id}"
cleanup() {
  art="${1:-}"
  if [ -z "${art}" ]; then
    log "FATAL" "Missing artifact"
  fi

  if [ ! -f "${SYSTEMD_PATH}/tomcat-${art}.service" ]; then
    return 0
  fi

  rc=$(root_call)

  # Clean.
  ${rc} "systemctl stop tomcat-${art}.service || true"
  ${rc} "rm -f ${SYSTEMD_PATH}/tomcat-${art}.service"
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
