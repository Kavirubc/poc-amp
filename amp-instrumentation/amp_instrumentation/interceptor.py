"""Tool interception and instrumentation for AI agents."""

import functools
import json
import uuid
import logging
from typing import Any, Callable, Dict, List, Optional
from datetime import datetime

import requests

logger = logging.getLogger(__name__)


class AMPInterceptor:
    """
    Intercepts tool calls, logs transactions, and enables compensation.

    Usage:
        interceptor = AMPInterceptor(
            agent_id="your-agent-id",
            amp_url="http://localhost:8080"
        )

        # Register tools
        interceptor.register_tools([
            {"name": "book_flight", "description": "Books a flight"},
            {"name": "cancel_flight", "description": "Cancels a flight"},
        ])

        # Wrap a tool function
        @interceptor.wrap_tool("book_flight")
        def book_flight(flight_id: str, passenger_id: str):
            # ... implementation
            return {"booking_id": "123"}
    """

    def __init__(
        self,
        agent_id: str,
        amp_url: str = "http://localhost:8080",
        session_id: Optional[str] = None,
        auto_register: bool = True,
    ):
        self.agent_id = agent_id
        self.amp_url = amp_url.rstrip("/")
        self.session_id = session_id or str(uuid.uuid4())
        self.auto_register = auto_register
        self.tools: Dict[str, Dict] = {}
        self.transaction_log: List[Dict] = []
        self._compensation_registry: Optional[Dict] = None

    def register_tools(self, tools: List[Dict]) -> Dict:
        """
        Register tools with the AMP platform for compensation analysis.

        Args:
            tools: List of tool schemas with name, description, and optional inputSchema

        Returns:
            Response from AMP with suggested compensation mappings
        """
        for tool in tools:
            self.tools[tool["name"]] = tool

        try:
            response = requests.post(
                f"{self.amp_url}/api/v1/agents/{self.agent_id}/tools",
                json={"tools": tools},
                timeout=30,
            )
            response.raise_for_status()
            result = response.json()
            logger.info(f"Registered {len(tools)} tools with AMP")
            return result
        except Exception as e:
            logger.warning(f"Failed to register tools with AMP: {e}")
            return {"error": str(e)}

    def get_compensation_registry(self, force_refresh: bool = False) -> Dict:
        """
        Get the approved compensation mappings from AMP.

        Returns:
            Dictionary mapping tool names to their compensators
        """
        if self._compensation_registry is not None and not force_refresh:
            return self._compensation_registry

        try:
            response = requests.get(
                f"{self.amp_url}/api/v1/agents/{self.agent_id}/compensation-mappings/approved",
                timeout=10,
            )
            response.raise_for_status()
            result = response.json()
            self._compensation_registry = result.get("registry", {})
            return self._compensation_registry
        except Exception as e:
            logger.warning(f"Failed to get compensation registry: {e}")
            return {}

    def log_execution(
        self,
        tool_name: str,
        input_params: Dict,
        output_result: Any,
    ) -> Optional[str]:
        """
        Log a tool execution to AMP for transaction tracking.

        Args:
            tool_name: Name of the tool that was executed
            input_params: Input parameters passed to the tool
            output_result: Result returned by the tool

        Returns:
            Transaction ID if successful, None otherwise
        """
        transaction = {
            "id": str(uuid.uuid4()),
            "tool_name": tool_name,
            "input": input_params,
            "output": output_result,
            "executed_at": datetime.utcnow().isoformat(),
        }
        self.transaction_log.append(transaction)

        try:
            response = requests.post(
                f"{self.amp_url}/api/v1/agents/{self.agent_id}/transactions",
                json={
                    "session_id": self.session_id,
                    "tool_name": tool_name,
                    "input": input_params,
                    "output": output_result if isinstance(output_result, dict) else {"result": output_result},
                },
                timeout=10,
            )
            response.raise_for_status()
            result = response.json()
            transaction["remote_id"] = result.get("transaction_id")
            logger.debug(f"Logged execution: {tool_name}")
            return result.get("transaction_id")
        except Exception as e:
            logger.warning(f"Failed to log execution to AMP: {e}")
            return None

    def wrap_tool(self, tool_name: str) -> Callable:
        """
        Decorator to wrap a tool function with interception.

        Usage:
            @interceptor.wrap_tool("book_flight")
            def book_flight(flight_id: str, passenger_id: str):
                return {"booking_id": "123"}
        """
        def decorator(func: Callable) -> Callable:
            @functools.wraps(func)
            def wrapper(*args, **kwargs):
                # Capture input
                input_params = kwargs.copy()
                if args:
                    # Try to map positional args to param names
                    import inspect
                    sig = inspect.signature(func)
                    param_names = list(sig.parameters.keys())
                    for i, arg in enumerate(args):
                        if i < len(param_names):
                            input_params[param_names[i]] = arg

                # Execute the tool
                try:
                    result = func(*args, **kwargs)

                    # Log successful execution
                    self.log_execution(tool_name, input_params, result)

                    return result
                except Exception as e:
                    # Log failed execution
                    self.log_execution(tool_name, input_params, {"error": str(e)})
                    raise

            return wrapper
        return decorator

    def get_rollback_plan(self) -> Dict:
        """
        Get a rollback plan for the current session.

        Returns:
            Rollback plan with steps to compensate executed tools
        """
        try:
            response = requests.get(
                f"{self.amp_url}/api/v1/agents/{self.agent_id}/sessions/{self.session_id}/rollback-plan",
                timeout=10,
            )
            response.raise_for_status()
            return response.json()
        except Exception as e:
            logger.error(f"Failed to get rollback plan: {e}")
            return {"error": str(e)}

    def execute_rollback(self, tool_executor: Optional[Callable[[str, Dict], Any]] = None) -> Dict:
        """
        Execute a rollback for the current session.

        Args:
            tool_executor: Optional callable that takes (tool_name, params) and executes the tool

        Returns:
            Rollback result with counts of compensated/skipped transactions
        """
        plan = self.get_rollback_plan()

        if "error" in plan:
            return plan

        results = {
            "total": len(plan.get("steps", [])),
            "compensated": 0,
            "skipped": 0,
            "failed": 0,
            "details": [],
        }

        for step in plan.get("steps", []):
            if step.get("action") == "skip":
                results["skipped"] += 1
                results["details"].append({
                    "tool": step["tool_name"],
                    "action": "skipped",
                    "reason": step.get("reason"),
                })
            elif step.get("action") == "compensate":
                if tool_executor:
                    try:
                        compensator = step.get("compensator_name")
                        params = step.get("compensation_params", {})
                        tool_executor(compensator, params)
                        results["compensated"] += 1
                        results["details"].append({
                            "tool": step["tool_name"],
                            "action": "compensated",
                            "compensator": compensator,
                        })
                    except Exception as e:
                        results["failed"] += 1
                        results["details"].append({
                            "tool": step["tool_name"],
                            "action": "failed",
                            "error": str(e),
                        })
                else:
                    # Just mark as compensated on the server side
                    results["compensated"] += 1
                    results["details"].append({
                        "tool": step["tool_name"],
                        "action": "compensated",
                        "compensator": step.get("compensator_name"),
                    })

        # Notify AMP of rollback
        try:
            requests.post(
                f"{self.amp_url}/api/v1/agents/{self.agent_id}/sessions/{self.session_id}/rollback",
                timeout=10,
            )
        except Exception as e:
            logger.warning(f"Failed to notify AMP of rollback: {e}")

        return results


def tool_wrapper(
    interceptor: AMPInterceptor,
    tool_name: str,
) -> Callable:
    """
    Standalone decorator for wrapping tool functions.

    Usage:
        @tool_wrapper(interceptor, "book_flight")
        def book_flight(flight_id: str):
            return {"booking_id": "123"}
    """
    return interceptor.wrap_tool(tool_name)
