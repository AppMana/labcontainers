"""JSON Schema object semantics shared by generated bindings.

JSON Schema permits additional properties unless explicitly forbidden. Pydantic's
default is to silently discard them, which would lose native component options.
Generated classes override this when their schema forbids additional properties.
"""

from pydantic import BaseModel, ConfigDict


class SchemaObject(BaseModel):
    model_config = ConfigDict(extra="allow")
