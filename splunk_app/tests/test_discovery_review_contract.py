import re
import unittest
from pathlib import Path


APP_ROOT = Path(__file__).resolve().parents[1] / "ping_monitor"


def stanza(text: str, name: str) -> str:
    match = re.search(
        rf"^\[{re.escape(name)}\]\s*$\n(.*?)(?=^\[|\Z)",
        text,
        flags=re.MULTILINE | re.DOTALL,
    )
    if match is None:
        raise AssertionError(f"missing stanza [{name}]")
    return match.group(1)


class DiscoveryReviewContractTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.macros = (APP_ROOT / "default" / "macros.conf").read_text(encoding="utf-8")
        cls.dashboard = (
            APP_ROOT / "default" / "data" / "ui" / "views" / "discovery_inventory.xml"
        ).read_text(encoding="utf-8")

    def test_discovery_normalization_labels_legacy_review_evidence(self) -> None:
        definition = stanza(self.macros, "ping_discovery_normalize")
        self.assertIn(
            'discovery_review_state=coalesce(discovery_review_state, "legacy_untracked")',
            definition,
        )

    def test_inventory_aggregation_preserves_review_and_addressing_fields(self) -> None:
        definition = stanza(self.macros, "ping_discovery_inventory(2)")
        for field in (
            "discovery_review_state",
            "discovery_reviewed_at",
            "discovery_review_note",
            "addressing_mode",
        ):
            self.assertIn(f"latest({field}) AS {field}", definition)

    def test_dashboard_review_filter_uses_fields_retained_by_macro(self) -> None:
        self.assertIn('discovery_review_state="$review_filter$"', self.dashboard)
        self.assertIn('discovery_review_state AS "Review State"', self.dashboard)
        self.assertIn('discovery_review_note AS "Review Note"', self.dashboard)


if __name__ == "__main__":
    unittest.main()
