import unittest
from app import can_view_order

class OrderAuthorizationTests(unittest.TestCase):
    def test_member_same_project_can_view(self):
        self.assertTrue(can_view_order("project-a", "project-a", True))

    def test_other_project_or_non_member_rejected(self):
        self.assertFalse(can_view_order("project-a", "project-b", True))
        self.assertFalse(can_view_order("project-a", "project-a", False))

if __name__ == "__main__":
    unittest.main()
