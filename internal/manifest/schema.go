package manifest

func Schema() []byte {
	return []byte(`{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "rungrid/v1",
  "title": "Rungrid workspace",
  "type": "object",
  "additionalProperties": false,
  "required": ["api_version", "kind", "project", "services"],
  "properties": {
    "api_version": {"const": "rungrid/v1"},
    "kind": {"const": "Workspace"},
    "project": {"$ref": "#/$defs/project"},
    "workspace": {"$ref": "#/$defs/workspace"},
    "repositories": {
      "type": "object",
      "propertyNames": {"pattern": "^[a-z][a-z0-9-]*$"},
      "additionalProperties": {"$ref": "#/$defs/repository"}
    },
    "imports": {"type": "array", "items": {"type": "string"}},
    "runtime": {"$ref": "#/$defs/runtime"},
    "terminal": {"$ref": "#/$defs/terminal"},
    "lifecycle": {"$ref": "#/$defs/lifecycle"},
    "services": {"type": "array", "items": {"$ref": "#/$defs/service"}}
  },
  "$defs": {
    "argv": {
      "type": "array",
      "minItems": 1,
      "items": {"type": "string", "minLength": 1}
    },
    "command": {
      "type": "object",
      "additionalProperties": false,
      "required": ["argv"],
      "properties": {"argv": {"$ref": "#/$defs/argv"}}
    },
    "project": {
      "type": "object",
      "additionalProperties": false,
      "required": ["name"],
      "properties": {
        "name": {"type": "string", "minLength": 1},
        "slug": {"type": "string", "pattern": "^[a-z0-9]+(?:-[a-z0-9]+)*$"},
        "id": {"type": "string", "pattern": "^[a-z0-9]+(?:-[a-z0-9]+)*-[a-z2-7]{6}$"}
      }
    },
    "workspace": {
      "type": "object",
      "additionalProperties": false,
      "properties": {"root": {"type": "string", "minLength": 1}}
    },
    "repository": {
      "type": "object",
      "additionalProperties": false,
      "required": ["path"],
      "properties": {
        "path": {"type": "string", "minLength": 1},
        "remote": {"type": "string", "pattern": "^[A-Za-z0-9][A-Za-z0-9._-]*$"},
        "default_branch": {"type": "string", "minLength": 1}
      }
    },
    "runtime": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "startup_timeout": {"type": "string"},
        "shutdown_timeout": {"type": "string"},
        "log_retention": {"type": "string"},
		"resource_guard": {"$ref": "#/$defs/resource_guard"},
        "process_compose": {
          "type": "object",
          "additionalProperties": false,
          "properties": {
            "executable": {"type": "string"},
            "log_level": {"enum": ["trace", "debug", "info", "warn", "error", "fatal", "panic", "disabled"]}
          }
        }
      }
    },
    "terminal": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "mode": {"enum": ["warp", "headless"]},
        "open": {"type": "boolean"},
        "theme": {"type": "string"}
      }
    },
	"adaptive_limit": {
	  "type": "object", "additionalProperties": false,
	  "properties": {"floor": {"type": "number", "exclusiveMinimum": 0}, "multiplier": {"type": "number", "minimum": 1}, "headroom": {"type": "number", "exclusiveMinimum": 0}}
	},
	"resource_limits": {
	  "type": "object", "additionalProperties": false,
	  "properties": {"cpu_percent": {"type": "number", "exclusiveMinimum": 0, "maximum": 100}, "memory_percent": {"type": "number", "exclusiveMinimum": 0, "maximum": 100}, "processes": {"type": "integer", "minimum": 1}, "threads": {"type": "integer", "minimum": 1}, "thread_growth": {"type": "integer", "minimum": 1}, "thread_growth_window": {"type": "string"}}
	},
	"adaptive_limits": {
	  "type": "object", "additionalProperties": false,
	  "properties": {"cpu": {"$ref": "#/$defs/adaptive_limit"}, "memory": {"$ref": "#/$defs/adaptive_limit"}, "processes": {"$ref": "#/$defs/adaptive_limit"}, "threads": {"$ref": "#/$defs/adaptive_limit"}, "thread_growth": {"type": "integer", "minimum": 1}, "thread_growth_window": {"type": "string"}}
	},
	"resource_guard": {
	  "type": "object", "additionalProperties": false,
	  "properties": {"sample_interval": {"type": "string"}, "learning_window": {"type": "string"}, "emergency_window": {"type": "string"}, "sustained_window": {"type": "string"}, "restart_limit": {"type": "integer", "minimum": 1}, "restart_window": {"type": "string"}, "backoff_initial": {"type": "string"}, "backoff_maximum": {"type": "string"}, "emergency": {"$ref": "#/$defs/resource_limits"}, "sustained": {"$ref": "#/$defs/adaptive_limits"}}
	},
    "provider": {
      "type": "object",
      "additionalProperties": false,
      "required": ["type"],
      "properties": {
        "type": {"enum": ["dotenv", "command", "direnv"]},
        "path": {"type": "string"},
        "optional": {"type": "boolean"},
        "argv": {"$ref": "#/$defs/argv"},
        "timeout": {"type": "string"},
        "directory": {"type": "string"}
      }
    },
    "environment": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "values": {"type": "object", "additionalProperties": {"type": "string"}},
        "providers": {"type": "array", "items": {"$ref": "#/$defs/provider"}}
      }
    },
    "lifecycle_command": {
      "type": "object",
      "additionalProperties": false,
      "required": ["name", "run"],
      "properties": {
        "name": {"type": "string", "pattern": "^[a-z][a-z0-9-]*$"},
        "working_directory": {"type": "string"},
        "timeout": {"type": "string"},
        "run": {"$ref": "#/$defs/command"},
        "environment": {"$ref": "#/$defs/environment"}
      }
    },
    "lifecycle": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "before_up": {"type": "array", "items": {"$ref": "#/$defs/lifecycle_command"}},
        "after_down": {"type": "array", "items": {"$ref": "#/$defs/lifecycle_command"}}
      }
    },
    "health": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "command": {"$ref": "#/$defs/command"},
        "url": {"type": "string", "format": "uri"},
        "interval": {"type": "string"},
        "timeout": {"type": "string"},
        "retries": {"type": "integer", "minimum": 1},
        "start_period": {"type": "string"}
      }
    },
    "service": {
      "type": "object",
      "additionalProperties": false,
      "required": ["name", "source"],
      "properties": {
        "name": {"type": "string", "pattern": "^[a-z][a-z0-9-]*$"},
        "repository": {"type": "string", "pattern": "^[a-z][a-z0-9-]*$"},
        "source": {"enum": ["native", "compose", "external"]},
        "activation": {"enum": ["workspace", "tab"]},
        "working_directory": {"type": "string"},
        "run": {
          "type": "object",
          "additionalProperties": false,
          "required": ["argv"],
          "properties": {"argv": {"$ref": "#/$defs/argv"}, "stdin": {"type": "boolean"}}
        },
        "compose": {
          "type": "object",
          "additionalProperties": false,
          "required": ["file", "service"],
          "properties": {
            "file": {"type": "string"},
            "project_name": {"type": "string"},
            "service": {"type": "string"},
            "profiles": {"type": "array", "items": {"type": "string"}},
            "up_argv": {"$ref": "#/$defs/argv"},
            "down_argv": {"$ref": "#/$defs/argv"}
          }
        },
        "external": {
          "type": "object",
          "additionalProperties": false,
          "properties": {"url": {"type": "string", "format": "uri"}, "command": {"$ref": "#/$defs/command"}}
        },
        "environment": {"$ref": "#/$defs/environment"},
        "depends_on": {"type": "object", "additionalProperties": {"enum": ["running", "healthy", "completed_successfully"]}},
        "health": {"$ref": "#/$defs/health"},
        "restart": {
          "type": "object",
          "additionalProperties": false,
          "properties": {
            "policy": {"enum": ["no", "always", "on-failure"]},
            "max_restarts": {"type": "integer", "minimum": 0},
            "backoff": {"type": "string"}
          }
        },
		"resource_guard": {"$ref": "#/$defs/resource_guard"},
        "namespace": {"type": "string"},
        "terminal": {
          "type": "object",
          "additionalProperties": false,
          "properties": {
            "title": {"type": "string"},
            "trigger_argv": {"$ref": "#/$defs/argv"},
            "include_in_versions": {"type": "boolean"}
          }
        },
        "ports": {"type": "array", "items": {"type": "integer", "minimum": 1, "maximum": 65535}}
      },
      "oneOf": [
        {"required": ["run"]},
        {"required": ["compose"]},
        {"required": ["external"]}
      ]
    }
  }
}
`)
}
