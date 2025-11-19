import React from 'react';
import BatchJobTable from '../../components/BatchJobTable';
import { Layout } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';

const BatchJob = () => {
  const { t } = useTranslation();
  return (
    <>
      <Layout>
        <Layout.Content>
          <BatchJobTable />
        </Layout.Content>
      </Layout>
    </>
  );
};

export default BatchJob;

