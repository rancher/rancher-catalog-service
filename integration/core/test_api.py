import pytest
import cattle


@pytest.fixture
def client():
    url = 'http://localhost:8088/v1-catalog/schemas'
    return cattle.from_env(url=url)


def test_catalog_list(client):
    templates = client.list_template()
    assert len(templates) > 0
