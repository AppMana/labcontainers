import json
import os
import sys
from vyos.configsession import ConfigSession

commands = json.load(sys.stdin)
session = ConfigSession(os.getpid())
try:
    for command in commands:
        if command["operation"] == "set":
            session.set(command["path"])
        elif command["operation"] == "delete":
            session.delete(command["path"])
        else:
            raise ValueError("unsupported operation")
    session.commit()
    session.save_config("/config/config.boot")
except Exception:
    session.discard()
    raise
print("LABCONTAINERS_VYOS_COMMITTED", flush=True)
